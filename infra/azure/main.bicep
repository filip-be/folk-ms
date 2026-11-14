// folk-ms infrastructure: one storage account (cms private, pub public,
// deployment), the CMS on Azure Functions Flex Consumption running as a
// user-assigned identity, two Key Vaults (platform secrets; page tokens,
// ADR 0010) with their audit logs and the storage writes in Log Analytics,
// alerts and a budget emailed to the Super Admin, and one Static Web App
// (Free) per Site. Deployed by .github/workflows/deploy-infra.yml.

targetScope = 'resourceGroup'

@description('Region of the storage account and the Function app.')
param location string = resourceGroup().location

@description('Region of the Static Web Apps (a region where SWA is offered).')
param swaLocation string = 'westeurope'

@description('Short prefix for resource names.')
@maxLength(8)
param prefix string = 'folkms'

@description('Site ids from config/sites.json; one Static Web App each.')
param siteIds array

@description('Every Site origin from config/sites.json (CORS on the pub container). Each Static Web App default origin is added to CORS automatically.')
param siteOrigins array

@description('Public origin of the CMS, e.g. https://cms.example.org. Empty: the Function app default host name.')
param cmsBaseUrl string = ''

@secure()
param superAdminEmail string
param googleClientId string
@secure()
param googleClientSecret string
param metaAppId string
@secure()
param metaAppSecret string
param metaGraphVersion string = 'v24.0'
@description('Base64 key (32+ bytes) that signs session cookies.')
@secure()
param sessionKey string

@description('Name of the CMS user-assigned managed identity in this resource group, created once in the first-deployment runbook (ADR 0010, T4).')
param cmsIdentityName string = '${prefix}-cms-id'

var suffix = uniqueString(resourceGroup().id)
var storageName = take('${prefix}${suffix}', 24)
var appName = '${prefix}-cms-${suffix}'
var vaultName = take('${prefix}-kv-${suffix}', 24)
var tokenVaultName = take('${prefix}-tok-${suffix}', 24)

// Created by the runbook, not here: the deploy identity's role-assignment
// condition names its principal id before the first deployment (T4), and it
// survives the Function app being recreated.
resource cmsIdentity 'Microsoft.ManagedIdentity/userAssignedIdentities@2023-01-31' existing = {
  name: cmsIdentityName
}

resource storage 'Microsoft.Storage/storageAccounts@2023-05-01' = {
  name: storageName
  location: location
  kind: 'StorageV2'
  sku: { name: 'Standard_LRS' }
  properties: {
    accessTier: 'Hot'
    allowBlobPublicAccess: true // only the pub container is public
    allowSharedKeyAccess: false // managed identity only
    minimumTlsVersion: 'TLS1_2'
    supportsHttpsTrafficOnly: true
    defaultToOAuthAuthentication: true
  }
}

resource blobService 'Microsoft.Storage/storageAccounts/blobServices@2023-05-01' = {
  parent: storage
  name: 'default'
  properties: {
    isVersioningEnabled: true
    deleteRetentionPolicy: { enabled: true, days: 30 }
    containerDeleteRetentionPolicy: { enabled: true, days: 30 }
    cors: {
      corsRules: [
        {
          allowedOrigins: siteOrigins
          allowedMethods: [ 'GET', 'HEAD' ]
          allowedHeaders: [ '*' ]
          exposedHeaders: [ 'ETag' ]
          maxAgeInSeconds: 3600
        }
        {
          // The *.azurestaticapps.net address of every Site, so a Site reads
          // its snapshot before (and without) a custom domain.
          allowedOrigins: [for (id, i) in siteIds: 'https://${sites[i].properties.defaultHostname}']
          allowedMethods: [ 'GET', 'HEAD' ]
          allowedHeaders: [ '*' ]
          exposedHeaders: [ 'ETag' ]
          maxAgeInSeconds: 3600
        }
      ]
    }
  }
}

resource cmsContainer 'Microsoft.Storage/storageAccounts/blobServices/containers@2023-05-01' = {
  parent: blobService
  name: 'cms'
  properties: { publicAccess: 'None' }
}

resource pubContainer 'Microsoft.Storage/storageAccounts/blobServices/containers@2023-05-01' = {
  parent: blobService
  name: 'pub'
  properties: { publicAccess: 'Blob' }
}

resource deployContainer 'Microsoft.Storage/storageAccounts/blobServices/containers@2023-05-01' = {
  parent: blobService
  name: 'deployment'
  properties: { publicAccess: 'None' }
}

// Old versions are only for recovery: keep them 90 days.
resource lifecycle 'Microsoft.Storage/storageAccounts/managementPolicies@2023-05-01' = {
  parent: storage
  name: 'default'
  properties: {
    policy: {
      rules: [
        {
          name: 'expire-old-versions'
          enabled: true
          type: 'Lifecycle'
          definition: {
            filters: { blobTypes: [ 'blockBlob' ] }
            actions: { version: { delete: { daysAfterCreationGreaterThan: 90 } } }
          }
        }
      ]
    }
  }
}

resource plan 'Microsoft.Web/serverfarms@2024-04-01' = {
  name: '${prefix}-plan-${suffix}'
  location: location
  kind: 'functionapp'
  sku: { tier: 'FlexConsumption', name: 'FC1' }
  properties: { reserved: true }
}

var baseUrl = empty(cmsBaseUrl) ? 'https://${appName}.azurewebsites.net' : cmsBaseUrl

resource app 'Microsoft.Web/sites@2024-04-01' = {
  name: appName
  location: location
  kind: 'functionapp,linux'
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: { '${cmsIdentity.id}': {} }
  }
  properties: {
    serverFarmId: plan.id
    httpsOnly: true
    keyVaultReferenceIdentity: cmsIdentity.id
    functionAppConfig: {
      deployment: {
        storage: {
          type: 'blobContainer'
          value: '${storage.properties.primaryEndpoints.blob}deployment'
          authentication: {
            type: 'UserAssignedIdentity'
            userAssignedIdentityResourceId: cmsIdentity.id
          }
        }
      }
      scaleAndConcurrency: {
        maximumInstanceCount: 40
        instanceMemoryMB: 512
      }
      runtime: { name: 'custom', version: '1.0' }
    }
    siteConfig: {
      minTlsVersion: '1.2'
      ftpsState: 'Disabled'
      http20Enabled: true
      appSettings: [
        { name: 'AzureWebJobsStorage__accountName', value: storage.name }
        { name: 'AzureWebJobsStorage__credential', value: 'managedidentity' }
        { name: 'AzureWebJobsStorage__clientId', value: cmsIdentity.properties.clientId }
        { name: 'AZURE_CLIENT_ID', value: cmsIdentity.properties.clientId }
        { name: 'KEYVAULT_URL', value: tokenVault.properties.vaultUri } // page tokens only
        { name: 'FOLKMS_ENV', value: 'azure' }
        { name: 'BASE_URL', value: baseUrl }
        { name: 'STORAGE_BLOB_URL', value: storage.properties.primaryEndpoints.blob }
        { name: 'STORAGE_PUBLIC_URL', value: '${storage.properties.primaryEndpoints.blob}pub' }
        { name: 'SUPER_ADMIN_EMAIL', value: superAdminEmail }
        { name: 'GOOGLE_CLIENT_ID', value: googleClientId }
        { name: 'GOOGLE_CLIENT_SECRET', value: googleClientSecret }
        { name: 'META_APP_ID', value: metaAppId }
        // Key Vault references, resolved by the platform as
        // keyVaultReferenceIdentity, so the values are not in app settings
        // or their exports (T2, T5).
        { name: 'META_APP_SECRET', value: '@Microsoft.KeyVault(SecretUri=${metaAppSecretKv.properties.secretUriWithVersion})' }
        { name: 'META_GRAPH_VERSION', value: metaGraphVersion }
        { name: 'SESSION_KEY', value: '@Microsoft.KeyVault(SecretUri=${sessionKeyKv.properties.secretUriWithVersion})' }
      ]
    }
  }
  // Roles first, so the host storage, the package and the Key Vault
  // references work on the first start.
  dependsOn: [ deployContainer, appBlobRole, appVaultRole, appTokenVaultRole ]
}

// The CMS identity reads and writes its data and the deployment package.
// Owner (not Contributor) because the Functions host keeps its own leases in
// the same account (AzureWebJobsStorage with identity requires it).
// The deploy identity may assign only this role, Key Vault Secrets User and
// Key Vault Secrets Officer, and only to the CMS identity (role-assignment
// condition, root README).
var blobDataOwner = 'b7e6dc6d-f1e8-4753-8033-0f276bb0955b'

resource appBlobRole 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  scope: storage
  name: guid(storage.id, cmsIdentity.id, blobDataOwner)
  properties: {
    principalId: cmsIdentity.properties.principalId
    principalType: 'ServicePrincipal'
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', blobDataOwner)
  }
}

// Two Key Vaults (ADR 0010), so the CMS identity holds the narrowest role
// on each (T2):
// - vault: the Meta app secret and SESSION_KEY, written here and read by
//   the platform for the Key Vault references below. The CMS identity can
//   only read (Key Vault Secrets User): it cannot overwrite, delete or purge
//   them.
// - tokenVault: the Facebook page tokens of Organizations with Autosync on,
//   one secret each, written, deleted and purged by the CMS (Key Vault
//   Secrets Officer on this vault only).
// RBAC only; the CMS identity is the only principal with a data role, and no
// human has a standing one (T1). Soft delete keeps a deleted secret 7 days,
// the minimum; the CMS purges a deleted token at once and sweeps leftovers
// daily (T11). Purge protection stays off: once on it cannot be turned off,
// and it would keep a leaked token recoverable. The API refuses false, so
// the property is left out.
resource vault 'Microsoft.KeyVault/vaults@2023-07-01' = {
  name: vaultName
  location: location
  properties: {
    tenantId: subscription().tenantId
    sku: { family: 'A', name: 'standard' }
    enableRbacAuthorization: true
    enableSoftDelete: true
    softDeleteRetentionInDays: 7
    publicNetworkAccess: 'Enabled' // Flex Consumption without a VNet
  }
}

resource tokenVault 'Microsoft.KeyVault/vaults@2023-07-01' = {
  name: tokenVaultName
  location: location
  properties: {
    tenantId: subscription().tenantId
    sku: { family: 'A', name: 'standard' }
    enableRbacAuthorization: true
    enableSoftDelete: true
    softDeleteRetentionInDays: 7
    publicNetworkAccess: 'Enabled'
  }
}

resource metaAppSecretKv 'Microsoft.KeyVault/vaults/secrets@2023-07-01' = {
  parent: vault
  name: 'meta-app-secret'
  properties: { value: metaAppSecret }
}

resource sessionKeyKv 'Microsoft.KeyVault/vaults/secrets@2023-07-01' = {
  parent: vault
  name: 'session-key'
  properties: { value: sessionKey }
}

// Read the two references above; nothing else.
var keyVaultSecretsUser = '4633458b-17de-408a-b874-0445c86b69e6'

resource appVaultRole 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  scope: vault
  name: guid(vault.id, cmsIdentity.id, keyVaultSecretsUser)
  properties: {
    principalId: cmsIdentity.properties.principalId
    principalType: 'ServicePrincipal'
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', keyVaultSecretsUser)
  }
}

// Get, set, list, delete and purge page tokens (Autosync on and off, the
// daily sweep).
var keyVaultSecretsOfficer = 'b86a8fe4-44ce-4948-aee5-eccb2c155cd7'

resource appTokenVaultRole 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  scope: tokenVault
  name: guid(tokenVault.id, cmsIdentity.id, keyVaultSecretsOfficer)
  properties: {
    principalId: cmsIdentity.properties.principalId
    principalType: 'ServicePrincipal'
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', keyVaultSecretsOfficer)
  }
}

// Detection (T8): both vaults' audit logs and the writes and deletes in the
// storage account go to a pay-as-you-go workspace (a few MB a month, inside
// the free allowance). Alerts email the Super Admin on: any vault operation
// by another identity; page token reads far above the normal pattern; a
// write or delete in the cms container by another identity; and changes to
// role assignments, the diagnostic settings, the alert rules or the vaults.
resource logs 'Microsoft.OperationalInsights/workspaces@2023-09-01' = {
  name: '${prefix}-logs-${suffix}'
  location: location
  properties: {
    sku: { name: 'PerGB2018' }
    retentionInDays: 30 // the free retention; incidents are emailed within the hour
    workspaceCapping: { dailyQuotaGb: 1 } // a cost guard; normal use is far below
  }
}

resource vaultDiagnostics 'Microsoft.Insights/diagnosticSettings@2021-05-01-preview' = {
  scope: vault
  name: 'audit-to-log-analytics'
  properties: {
    workspaceId: logs.id
    logs: [ { category: 'AuditEvent', enabled: true } ]
  }
}

resource tokenVaultDiagnostics 'Microsoft.Insights/diagnosticSettings@2021-05-01-preview' = {
  scope: tokenVault
  name: 'audit-to-log-analytics'
  properties: {
    workspaceId: logs.id
    logs: [ { category: 'AuditEvent', enabled: true } ]
  }
}

// Writes and deletes only (reads would be most of the volume): who changed
// a CMS document or the Audit Log, for the alert below and for forensics.
resource blobDiagnostics 'Microsoft.Insights/diagnosticSettings@2021-05-01-preview' = {
  scope: blobService
  name: 'writes-to-log-analytics'
  properties: {
    workspaceId: logs.id
    logs: [
      { category: 'StorageWrite', enabled: true }
      { category: 'StorageDelete', enabled: true }
    ]
  }
}

resource superAdminAlerts 'Microsoft.Insights/actionGroups@2023-01-01' = {
  name: '${prefix}-super-admin-${suffix}'
  location: 'global'
  properties: {
    groupShortName: '${prefix}-adm' // at most 12 characters
    enabled: true
    emailReceivers: [
      { name: 'super-admin', emailAddress: superAdminEmail, useCommonAlertSchema: true }
    ]
  }
}

// AzureDiagnostics keeps the caller's object id in one of two columns,
// depending on the token version; column_ifexists covers both. Every
// operation by another identity counts (reads, lists, backups, restores,
// recovers, purges, sets and deletes), on either vault. Reads of page
// tokens count against the hourly threshold. The tables exist only after
// their first event, hence isfuzzy. Multi-line strings do not interpolate,
// hence replace().
var maxSecretGetsPerHour = 100
var unusualAccessQuery = replace(replace(replace('''
let cms = tolower('CMS_PRINCIPAL_ID');
let kv = AzureDiagnostics
  | where ResourceProvider == "MICROSOFT.KEYVAULT" and Category == "AuditEvent"
  | extend caller = tolower(coalesce(
      tostring(column_ifexists("identity_claim_oid_g", "")),
      tostring(column_ifexists("identity_claim_http_schemas_microsoft_com_identity_claims_objectidentifier_g", ""))))
  | extend vaultName = tolower(Resource);
let foreign = kv
  | where isnotempty(caller) and caller != cms
  | summarize reads = count() by caller
  | extend reason = "Key Vault used by another identity";
let many = kv
  | where vaultName == tolower('TOKEN_VAULT') and OperationName == "SecretGet"
  | summarize reads = count()
  | where reads > MAX_GETS
  | extend caller = "", reason = "more than MAX_GETS page token reads in an hour";
let blobs = union isfuzzy=true (StorageBlobLogs | where ObjectKey has "/cms/" | extend caller = tolower(RequesterObjectId))
  | where isnotempty(caller) and caller != cms
  | summarize reads = count() by caller
  | extend reason = "cms container written by another identity";
union isfuzzy=true foreign, many, blobs
''', 'CMS_PRINCIPAL_ID', cmsIdentity.properties.principalId), 'MAX_GETS', string(maxSecretGetsPerHour)), 'TOKEN_VAULT', tokenVaultName)

resource unusualReadsAlert 'Microsoft.Insights/scheduledQueryRules@2023-03-15-preview' = {
  name: '${prefix}-kv-unusual-reads-${suffix}'
  location: location
  properties: {
    displayName: 'folk-ms: unusual secret or data access'
    description: 'A Key Vault was used, or the cms container written, by an identity other than the CMS, or page tokens were read more than ${maxSecretGetsPerHour} times in an hour. See "Key Vault and token incident response" in the README.'
    severity: 1
    enabled: true
    scopes: [ logs.id ]
    // Every 15 minutes over the last hour: audit events reach the workspace
    // minutes late, so an hourly window evaluated hourly could miss them.
    // At 10 minutes or more the rule costs about $0.50 a month.
    evaluationFrequency: 'PT15M'
    windowSize: 'PT1H'
    muteActionsDuration: 'PT1H' // one email an hour at most
    autoMitigate: false
    // The tables exist only after the first event arrives.
    skipQueryValidation: true
    criteria: {
      allOf: [
        {
          query: unusualAccessQuery
          timeAggregation: 'Count'
          operator: 'GreaterThan'
          threshold: 0
          failingPeriods: { numberOfEvaluationPeriods: 1, minFailingPeriodsToAlert: 1 }
        }
      ]
    }
    actions: { actionGroups: [ superAdminAlerts.id ] }
  }
  dependsOn: [ vaultDiagnostics, tokenVaultDiagnostics, blobDiagnostics ]
}

// Free (Activity Log alerts cost nothing): someone changing who may read
// the secrets, or switching detection off. The deploy pipeline's own
// changes also notify, which is expected after a deployment.
resource controlPlaneAlert 'Microsoft.Insights/activityLogAlerts@2020-10-01' = {
  name: '${prefix}-control-plane-${suffix}'
  location: 'global'
  properties: {
    enabled: true
    description: 'Role assignments, diagnostic settings, alert rules or Key Vaults in the folk-ms resource group changed. See "Key Vault and token incident response" in the README.'
    scopes: [ resourceGroup().id ]
    condition: {
      allOf: [
        { field: 'category', equals: 'Administrative' }
        { field: 'status', equals: 'Succeeded' }
        {
          anyOf: [
            { field: 'operationName', equals: 'Microsoft.Authorization/roleAssignments/write' }
            { field: 'operationName', equals: 'Microsoft.Authorization/roleAssignments/delete' }
            { field: 'operationName', equals: 'Microsoft.Insights/diagnosticSettings/delete' }
            { field: 'operationName', equals: 'Microsoft.Insights/scheduledQueryRules/delete' }
            { field: 'operationName', equals: 'Microsoft.Insights/activityLogAlerts/delete' }
            { field: 'operationName', equals: 'Microsoft.KeyVault/vaults/delete' }
            { field: 'operationName', equals: 'Microsoft.KeyVault/vaults/accessPolicies/write' }
          ]
        }
      ]
    }
    actions: { actionGroups: [ { actionGroupId: superAdminAlerts.id } ] }
  }
}

// A cost guard (T15): Flex Consumption cannot scale below 40 instances, so
// a flood of requests could pass the free grant. The Super Admin is emailed
// at half and at all of the monthly budget.
@description('Monthly budget in the billing currency; the Super Admin is emailed at 50% and 100%.')
param monthlyBudget int = 10

@description('First day of the budget period (yyyy-MM-01); defaults to this month.')
param budgetStart string = utcNow('yyyy-MM-01')

resource budget 'Microsoft.Consumption/budgets@2023-11-01' = {
  name: '${prefix}-budget'
  properties: {
    category: 'Cost'
    amount: monthlyBudget
    timeGrain: 'Monthly'
    timePeriod: { startDate: budgetStart }
    notifications: {
      half: { enabled: true, operator: 'GreaterThan', threshold: 50, thresholdType: 'Actual', contactEmails: [ superAdminEmail ] }
      all: { enabled: true, operator: 'GreaterThan', threshold: 100, thresholdType: 'Actual', contactEmails: [ superAdminEmail ] }
      forecast: { enabled: true, operator: 'GreaterThan', threshold: 100, thresholdType: 'Forecasted', contactEmails: [ superAdminEmail ] }
    }
  }
}

resource sites 'Microsoft.Web/staticSites@2024-04-01' = [for id in siteIds: {
  name: '${prefix}-${id}'
  location: swaLocation
  tags: { 'folkms-site': id } // deploy-site.yml finds the app by this tag
  sku: { name: 'Free', tier: 'Free' }
  properties: {
    allowConfigFileUpdates: true
    stagingEnvironmentPolicy: 'Disabled'
  }
}]

output storageAccount string = storage.name
output blobEndpoint string = storage.properties.primaryEndpoints.blob
output functionApp string = app.name
output keyVault string = vault.name
output keyVaultUrl string = vault.properties.vaultUri
output tokenVault string = tokenVault.name
output tokenVaultUrl string = tokenVault.properties.vaultUri
output cmsIdentityClientId string = cmsIdentity.properties.clientId
output cmsUrl string = baseUrl
output staticSites array = [for (id, i) in siteIds: {
  id: id
  name: sites[i].name
  hostname: sites[i].properties.defaultHostname
}]

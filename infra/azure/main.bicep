// folk-ms infrastructure: one storage account (cms private, pub public,
// deployment), the CMS on Azure Functions Flex Consumption running as a
// user-assigned identity, a Key Vault for secrets (ADR 0010) with its audit
// log in Log Analytics and an alert to the Super Admin, and one Static Web
// App (Free) per Site. Deployed by .github/workflows/deploy-infra.yml.

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
        { name: 'KEYVAULT_URL', value: vault.properties.vaultUri }
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
  dependsOn: [ deployContainer, appBlobRole, appVaultRole ]
}

// The CMS identity reads and writes its data and the deployment package.
// Owner (not Contributor) because the Functions host keeps its own leases in
// the same account (AzureWebJobsStorage with identity requires it).
// The deploy identity may assign only this role and Key Vault Secrets Officer,
// and only to the CMS identity (role-assignment condition, root README).
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

// Key Vault (ADR 0010): the Facebook page tokens of Organizations with
// Autosync on (one secret each, written by the CMS), the Meta app secret and
// SESSION_KEY (written here). RBAC only; the CMS identity is the only
// principal with a data role, and no human has a standing one (T1).
// Soft delete keeps a deleted secret 7 days, the minimum (T11). Purge
// protection stays off: the CMS never reuses a secret name, so it needs no
// purge right, and an Owner can purge a leaked secret at once. Once on it
// cannot be turned off, and the API refuses false, so the property is left
// out.
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

// Get, set and delete secrets (Autosync on and off), and read the two
// references above.
var keyVaultSecretsOfficer = 'b86a8fe4-44ce-4948-aee5-eccb2c155cd7'

resource appVaultRole 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  scope: vault
  name: guid(vault.id, cmsIdentity.id, keyVaultSecretsOfficer)
  properties: {
    principalId: cmsIdentity.properties.principalId
    principalType: 'ServicePrincipal'
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', keyVaultSecretsOfficer)
  }
}

// Detection (T8): the vault's audit log goes to a pay-as-you-go workspace
// (a few MB a month, inside the free allowance), and an alert emails the
// Super Admin when another identity reads a secret or reads are far above
// the hourly Sync pattern.
resource logs 'Microsoft.OperationalInsights/workspaces@2023-09-01' = {
  name: '${prefix}-logs-${suffix}'
  location: location
  properties: {
    sku: { name: 'PerGB2018' }
    retentionInDays: 30
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
// depending on the token version; column_ifexists covers both. The two
// secrets App Service reads for the references above are left out of the
// count (each start reads them); a read by another identity counts for
// every secret. Multi-line strings do not interpolate, hence replace().
var maxSecretGetsPerHour = 30
var unusualReadsQuery = replace(replace('''
let cms = tolower('CMS_PRINCIPAL_ID');
let gets = AzureDiagnostics
  | where ResourceProvider == "MICROSOFT.KEYVAULT" and Category == "AuditEvent" and OperationName == "SecretGet"
  | extend caller = tolower(coalesce(
      tostring(column_ifexists("identity_claim_oid_g", "")),
      tostring(column_ifexists("identity_claim_http_schemas_microsoft_com_identity_claims_objectidentifier_g", ""))))
  | extend secret = extract(@"/secrets/([^/?]+)", 1, tostring(column_ifexists("id_s", "")));
let foreign = gets
  | where isnotempty(caller) and caller != cms
  | summarize reads = count() by caller
  | extend reason = "secret read by another identity";
let many = gets
  | where secret !in ("meta-app-secret", "session-key")
  | summarize reads = count()
  | where reads > MAX_GETS
  | extend caller = "", reason = "more than MAX_GETS secret reads in an hour";
union foreign, many
''', 'CMS_PRINCIPAL_ID', cmsIdentity.properties.principalId), 'MAX_GETS', string(maxSecretGetsPerHour))

resource unusualReadsAlert 'Microsoft.Insights/scheduledQueryRules@2023-03-15-preview' = {
  name: '${prefix}-kv-unusual-reads-${suffix}'
  location: location
  properties: {
    displayName: 'folk-ms: unusual Key Vault secret reads'
    description: 'A Key Vault secret was read by an identity other than the CMS, or more than ${maxSecretGetsPerHour} times in an hour. See "Key Vault and token incident response" in the README.'
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
    // AzureDiagnostics exists only after the first audit event arrives.
    skipQueryValidation: true
    criteria: {
      allOf: [
        {
          query: unusualReadsQuery
          timeAggregation: 'Count'
          operator: 'GreaterThan'
          threshold: 0
          failingPeriods: { numberOfEvaluationPeriods: 1, minFailingPeriodsToAlert: 1 }
        }
      ]
    }
    actions: { actionGroups: [ superAdminAlerts.id ] }
  }
  dependsOn: [ vaultDiagnostics ]
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
output cmsIdentityClientId string = cmsIdentity.properties.clientId
output cmsUrl string = baseUrl
output staticSites array = [for (id, i) in siteIds: {
  id: id
  name: sites[i].name
  hostname: sites[i].properties.defaultHostname
}]

// folk-ms infrastructure: one storage account (cms private, pub public,
// deployment), the CMS on Azure Functions Flex Consumption with a
// system-assigned identity, and one Static Web App (Free) per Site.
// Deployed by .github/workflows/deploy-infra.yml.

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

var suffix = uniqueString(resourceGroup().id)
var storageName = take('${prefix}${suffix}', 24)
var appName = '${prefix}-cms-${suffix}'

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
  identity: { type: 'SystemAssigned' }
  properties: {
    serverFarmId: plan.id
    httpsOnly: true
    functionAppConfig: {
      deployment: {
        storage: {
          type: 'blobContainer'
          value: '${storage.properties.primaryEndpoints.blob}deployment'
          authentication: { type: 'SystemAssignedIdentity' }
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
        { name: 'FOLKMS_ENV', value: 'azure' }
        { name: 'BASE_URL', value: baseUrl }
        { name: 'STORAGE_BLOB_URL', value: storage.properties.primaryEndpoints.blob }
        { name: 'STORAGE_PUBLIC_URL', value: '${storage.properties.primaryEndpoints.blob}pub' }
        { name: 'SUPER_ADMIN_EMAIL', value: superAdminEmail }
        { name: 'GOOGLE_CLIENT_ID', value: googleClientId }
        { name: 'GOOGLE_CLIENT_SECRET', value: googleClientSecret }
        { name: 'META_APP_ID', value: metaAppId }
        { name: 'META_APP_SECRET', value: metaAppSecret }
        { name: 'META_GRAPH_VERSION', value: metaGraphVersion }
        { name: 'SESSION_KEY', value: sessionKey }
      ]
    }
  }
  dependsOn: [ deployContainer ]
}

// The app's identity reads and writes its data and the deployment package.
// Owner (not Contributor) because the Functions host keeps its own leases in
// the same account (AzureWebJobsStorage with identity requires it).
var blobDataOwner = 'b7e6dc6d-f1e8-4753-8033-0f276bb0955b'

resource appBlobRole 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  scope: storage
  name: guid(storage.id, app.id, blobDataOwner)
  properties: {
    principalId: app.identity.principalId
    principalType: 'ServicePrincipal'
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', blobDataOwner)
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
output cmsUrl string = baseUrl
output staticSites array = [for (id, i) in siteIds: {
  id: id
  name: sites[i].name
  hostname: sites[i].properties.defaultHostname
}]

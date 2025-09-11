# Kong Gateway Sample Configurations

This directory contains sample Kubernetes resources and Helm configurations for testing the Kong Gateway connector with WSO2 API Manager.

## 📁 Directory Structure

```
kong/sample/
├── README.md           # This file
├── api_crs/           # Kubernetes API Custom Resources
│   ├── comments-acl.yaml
│   ├── comments-cors.yaml
│   ├── comments-jwt.yaml
│   ├── comments-route-1-options.yaml
│   ├── comments-route-1.yaml
│   └── comments-service.yaml
└── helm/              # Helm chart values
    └── values.yaml
```

## 🚀 Sample API: Comments API

The sample configurations demonstrate a complete API setup with authentication, authorization, and CORS support.

### API Overview
- **API Name**: Comments API
- **Base Path**: `/comments/*`
- **Backend**: Mock service at `68870560071f195ca97eed8a.mockapi.io`
- **Authentication**: JWT-based authentication
- **Authorization**: ACL-based access control
- **CORS**: Cross-Origin Resource Sharing enabled

## 🔧 Kubernetes Resources

### 1. HTTPRoute (`comments-route-1.yaml`)

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  annotations:
    konghq.com/plugins: jwt-comments-api-plugin,acl-comments-api-plugin,cors-comments-api-plugin
    konghq.com/strip-path: "true"
  name: comments-api-route
  namespace: kong
```

**Features:**
- Multiple Kong plugins attached via annotations
- Path stripping enabled
- Request header modification for backend routing
- URL rewrite capabilities

### 2. Service (`comments-service.yaml`)

Kubernetes Service resource that defines the backend service endpoint for the Comments API.

### 3. Kong Plugins

#### JWT Plugin (`comments-jwt.yaml`)
```yaml
apiVersion: configuration.konghq.com/v1
config:
  claims_to_verify: [exp]
  header_names: [Authorization]
  key_claim_name: client_id
  run_on_preflight: false
```

**Purpose:** JWT token validation and authentication
- Verifies JWT expiration (`exp` claim)
- Extracts client ID from `client_id` claim
- Expects JWT in `Authorization` header

#### ACL Plugin (`comments-acl.yaml`)
```yaml
apiVersion: configuration.konghq.com/v1
config:
  allow: [api-comments-production-new]
plugin: acl
```

**Purpose:** Access Control List for authorization
- Only allows access to users in `api-comments-production-new` group
- Works in conjunction with JWT plugin for fine-grained access control

#### CORS Plugin (`comments-cors.yaml`)
```yaml
apiVersion: configuration.konghq.com/v1
config:
  credentials: false
  headers: [authorization, Access-Control-Allow-Origin, Content-Type, ...]
  methods: [GET, PUT, POST, DELETE, PATCH, OPTIONS]
  origins: ['*']
```

**Purpose:** Cross-Origin Resource Sharing configuration
- Allows all origins (`*`)
- Supports common HTTP methods
- Includes necessary headers for API access

### 4. OPTIONS Route (`comments-route-1-options.yaml`)

Dedicated HTTPRoute for handling CORS preflight OPTIONS requests.

## ⚙️ Helm Configuration

### Values (`helm/values.yaml`)

```yaml
wso2:
  subscription:
    imagePullSecrets: ""
replicaCount: 1
```

**Configuration Options:**
- Image pull secrets for private registries
- Replica count for high availability
- Additional WSO2-specific configurations

## 🧪 Testing the Configuration

### Prerequisites

1. **Kong Gateway** installed in Kubernetes cluster
2. **Kong Gateway Connector** deployed
3. **WSO2 API Manager** configured

### Deployment Steps

1. **Apply Kubernetes resources:**
   ```bash
   kubectl apply -f api_crs/
   ```

2. **Deploy with Helm:**
   ```bash
   helm install kong-agent common-agent/helm/. -f kong/sample/helm/values.yaml
   ```

3. **Verify deployment:**
   ```bash
   kubectl get httproutes,services,kongplugins -n kong
   ```

## 🔍 Troubleshooting

### Common Issues

1. **JWT Authentication Failures**
   - Verify JWT token is valid and not expired
   - Check `client_id` claim is present in JWT
   - Ensure JWT is sent in `Authorization` header

2. **ACL Authorization Failures**
   - Confirm user belongs to `api-comments-production-new` group
   - Verify ACL plugin configuration
   - Check Kong Consumer has correct ACL credentials

3. **CORS Issues**
   - Verify CORS plugin is attached to HTTPRoute
   - Check origin is allowed in CORS configuration
   - Ensure OPTIONS route is properly configured

### Logs and Debugging

```bash
# Check Kong Gateway logs
kubectl logs -n kong deployment/kong-gateway

# Check Gateway Agent logs
kubectl logs -n kong deployment/kong-agent-wso2-common-agent-deployment

# Verify plugin configurations
kubectl describe kongplugin -n kong
```

## 🔧 Customization

### Modifying the Sample

1. **Change Backend Service:**
   - Update `comments-service.yaml` with your backend details
   - Modify `comments-route-1.yaml` backend references

2. **Update Authentication:**
   - Modify JWT plugin configuration in `comments-jwt.yaml`
   - Adjust claim verification and header settings

3. **Configure ACL Groups:**
   - Update `allow` list in `comments-acl.yaml`

4. **Customize CORS:**
   - Modify origins, methods, and headers in `comments-cors.yaml`
   - Add or remove allowed domains as needed

## 📚 Related Documentation

- [Kong Gateway Documentation](https://docs.konghq.com/gateway/)
- [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/)
- [WSO2 API Manager Documentation](https://apim.docs.wso2.com/)
- [Kong Plugin Hub](https://docs.konghq.com/hub/)

## 🤝 Contributing

When modifying sample configurations:

1. Test changes in a development environment
2. Update documentation accordingly
3. Ensure backward compatibility
4. Add appropriate comments to YAML files

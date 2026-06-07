# Install Traefik ingress controller via Helm.
# Run once per cluster. k3s production already ships Traefik — skip on k3s.
#
# Prerequisites: helm CLI installed (https://helm.sh/docs/intro/install/)
#
# After install, set NODE_IP in web-frontend deployment:
#   - Docker Desktop local: NODE_IP = localhost
#   - k3s production:       NODE_IP = your node's external IP or domain

param(
    [string]$NodeIP = "localhost"
)

Write-Host "Adding Traefik Helm repo..."
helm repo add traefik https://traefik.github.io/charts
helm repo update

Write-Host "Installing Traefik..."
helm upgrade --install traefik traefik/traefik `
    --namespace traefik `
    --create-namespace `
    --set ports.web.port=80 `
    --set ports.web.expose.default=true `
    --set ports.websecure.expose.default=false `
    --set service.type=LoadBalancer `
    --wait

Write-Host "Traefik installed. Patching web-frontend with NODE_IP=$NodeIP..."
kubectl set env deployment/web-frontend -n superstreaming `
    READ_BASE_URLS="http://$NodeIP/whep/0/,http://$NodeIP/whep/1/,http://$NodeIP/whep/2/" `
    API_SERVER_URL="http://$NodeIP/api"

Write-Host "Applying SuperStreaming ingress rules..."
kubectl apply -f "$PSScriptRoot\..\k8s\traefik\middlewares.yaml"
kubectl apply -f "$PSScriptRoot\..\k8s\traefik\ingressroutes.yaml"

Write-Host ""
Write-Host "Done. Open http://$NodeIP in your browser." -ForegroundColor Green

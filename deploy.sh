#!/bin/bash

# Production Deployment Script
# This script deploys the microservices application in production mode

set -e  # Exit on any error

COMPOSE_FILE="deploy/docker-compose.prod.yml"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if Docker and Docker Compose are installed
check_dependencies() {
    print_status "Checking dependencies..."
    
    if ! command -v docker &> /dev/null; then
        print_error "Docker is not installed or not in PATH"
        exit 1
    fi
    
    if ! command -v docker compose &> /dev/null && ! docker compose version &> /dev/null; then
        print_error "Docker Compose is not installed or not in PATH"
        exit 1
    fi
    
    print_success "Dependencies check passed"
}

# Set production environment variables
setup_environment() {
    print_status "Setting up production environment..."
    
    # Export production environment variables
    export COMPOSE_PROJECT_NAME="microservices-prod"
    export NODE_ENV="production"
    export GO_ENV="production"
    
    # Create .env file if it doesn't exist
    if [ ! -f .env ]; then
        print_warning ".env file not found, creating with default values..."
        cat > .env << EOF
# Production Environment Variables
MONGO_INITDB_ROOT_USERNAME=admin
MONGO_INITDB_ROOT_PASSWORD=$(openssl rand -hex 32)
MONGO_INITDB_DATABASE=auth_db
JWT_SECRET=$(openssl rand -hex 64)
LOG_LEVEL=0
API_GATEWAY_PORT=8080
FRONTEND_PORT=8085
# HTTPS cookie settings — set COOKIE_SECURE=false and COOKIE_SAME_SITE=Lax only for non-HTTPS deployments
COOKIE_SECURE=true
COOKIE_SAME_SITE=None
# Comma-separated origins allowed by auth/user services (set your real frontend URL)
ALLOWED_ORIGINS=http://localhost:8085
EOF
        print_success "Created .env file with secure random values"
    fi

    # Compose reads .env from the project directory (repo root)
    set -a
    # shellcheck disable=SC1091
    source .env
    set +a
    
    print_success "Production environment setup complete"
}

# Backup existing data if containers exist
backup_data() {
    print_status "Checking for existing containers..."
    
    if docker ps -a --format "table {{.Names}}" | grep -q "mongodb\|auth-service\|user-service\|api-gateway"; then
        print_warning "Existing containers found. Creating backup..."
        
        BACKUP_DIR="backups/$(date +%Y%m%d_%H%M%S)"
        mkdir -p "$BACKUP_DIR"
        
        # Backup MongoDB data if exists
        if docker ps --format "table {{.Names}}" | grep -q "mongodb"; then
            print_status "Backing up MongoDB data..."
            docker exec mongodb mongodump --out /tmp/backup
            docker cp mongodb:/tmp/backup "$BACKUP_DIR/mongodb"
        fi
        
        print_success "Backup created at $BACKUP_DIR"
    fi
}

# Build and deploy services
deploy_services() {
    print_status "Deploying microservices..."
    
    # Stop existing services
    print_status "Stopping existing services..."
    docker compose -f "$COMPOSE_FILE" --env-file .env down --remove-orphans || true
    
    # Build and start services
    print_status "Building and starting services..."
    docker compose -f "$COMPOSE_FILE" --env-file .env up -d --build
    
    print_success "Services deployed successfully"
}

# Wait for services to be healthy
wait_for_services() {
    print_status "Waiting for services to be healthy..."
    
    services=("mongodb" "auth-service" "user-service" "api-gateway" "frontend")
    max_attempts=30
    attempt=1
    
    for service in "${services[@]}"; do
        print_status "Waiting for $service to be healthy..."
        
        while [ $attempt -le $max_attempts ]; do
            if docker compose -f "$COMPOSE_FILE" --env-file .env ps "$service" | grep -q "healthy\|Up"; then
                print_success "$service is healthy"
                break
            fi
            
            if [ $attempt -eq $max_attempts ]; then
                print_error "$service failed to become healthy within ${max_attempts} attempts"
                docker compose -f "$COMPOSE_FILE" --env-file .env logs "$service"
                exit 1
            fi
            
            print_status "Attempt $attempt/$max_attempts for $service..."
            sleep 10
            ((attempt++))
        done
        attempt=1
    done
    
    print_success "All services are healthy and running"
}

# Run health checks (only host-published endpoints)
run_health_checks() {
    print_status "Running comprehensive health checks..."

    local gateway_port="${API_GATEWAY_PORT:-8080}"
    local frontend_port="${FRONTEND_PORT:-8085}"
    
    if curl -f "http://localhost:${gateway_port}/health" > /dev/null 2>&1; then
        print_success "API Gateway health check passed"
    else
        print_error "API Gateway health check failed"
        exit 1
    fi
    
    if curl -f "http://localhost:${frontend_port}" > /dev/null 2>&1; then
        print_success "Frontend health check passed"
    else
        print_error "Frontend health check failed"
        exit 1
    fi
    
    print_success "All health checks passed"
}

# Display deployment information
show_deployment_info() {
    local gateway_port="${API_GATEWAY_PORT:-8080}"
    local frontend_port="${FRONTEND_PORT:-8085}"

    print_success "Deployment completed successfully!"
    echo ""
    echo "=== Production Deployment Information ==="
    echo "API Gateway:     http://localhost:${gateway_port}"
    echo "Swagger UI:      http://localhost:${gateway_port}/swagger/"
    echo "Frontend:        http://localhost:${frontend_port}"
    echo "Auth/User/Mongo/Kafka: internal Docker network only"
    echo ""
    echo "=== Useful Commands ==="
    echo "View logs:       docker compose -f ${COMPOSE_FILE} --env-file .env logs -f"
    echo "Check status:    docker compose -f ${COMPOSE_FILE} --env-file .env ps"
    echo "Stop services:   docker compose -f ${COMPOSE_FILE} --env-file .env down"
    echo "Restart service: docker compose -f ${COMPOSE_FILE} --env-file .env restart [service-name]"
    echo ""
    echo "=== Monitoring ==="
    echo "Monitor health:  watch docker compose -f ${COMPOSE_FILE} --env-file .env ps"
    echo "View resources:  docker stats"
}

# Cleanup function for graceful shutdown
cleanup() {
    print_status "Performing cleanup..."
    # Add any cleanup tasks here
}

# Main execution
main() {
    print_status "Starting production deployment..."
    
    # Trap to ensure cleanup on exit
    trap cleanup EXIT
    
    check_dependencies
    setup_environment
    backup_data
    deploy_services
    wait_for_services
    run_health_checks
    show_deployment_info
    
    print_success "Production deployment completed successfully!"
}

# Handle script arguments
case "${1:-deploy}" in
    "deploy")
        main
        ;;
    "stop")
        print_status "Stopping production services..."
        if [ -f .env ]; then set -a; # shellcheck disable=SC1091
            source .env; set +a; fi
        docker compose -f "$COMPOSE_FILE" --env-file .env down
        print_success "Services stopped"
        ;;
    "restart")
        print_status "Restarting production services..."
        if [ -f .env ]; then set -a; # shellcheck disable=SC1091
            source .env; set +a; fi
        docker compose -f "$COMPOSE_FILE" --env-file .env restart
        print_success "Services restarted"
        ;;
    "logs")
        docker compose -f "$COMPOSE_FILE" --env-file .env logs -f
        ;;
    "status")
        docker compose -f "$COMPOSE_FILE" --env-file .env ps
        ;;
    "health")
        if [ -f .env ]; then set -a; # shellcheck disable=SC1091
            source .env; set +a; fi
        run_health_checks
        ;;
    "help"|"-h"|"--help")
        echo "Usage: $0 [command]"
        echo ""
        echo "Commands:"
        echo "  deploy   Deploy the application (default)"
        echo "  stop     Stop all services"
        echo "  restart  Restart all services"
        echo "  logs     Show and follow logs"
        echo "  status   Show service status"
        echo "  health   Run health checks"
        echo "  help     Show this help message"
        ;;
    *)
        print_error "Unknown command: $1"
        echo "Use '$0 help' for available commands"
        exit 1
        ;;
esac

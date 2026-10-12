BACKEND_SERVICES := auth-service user-service notification-service api-gateway

.PHONY: lint-format test security-check trivy-security-check trivy-fs-security-check trivy-image-security-check

# Check formatting and run go vet for every backend service.
lint-format:
	bash scripts/pre-commit.sh --all

# Run unit tests for every backend service.
test:
	@set -e; \
	for service in $(BACKEND_SERVICES); do \
		echo "Testing $$service..."; \
		(cd "$$service" && go test -v ./...); \
	done

# Scan every backend service for known Go and image vulnerabilities.
security-check:
	@if ! command -v govulncheck >/dev/null 2>&1; then \
		echo "error: govulncheck is required — install it with:" >&2; \
		echo "  go install golang.org/x/vuln/cmd/govulncheck@latest" >&2; \
		exit 1; \
	fi
	@status=0; \
	for service in $(BACKEND_SERVICES); do \
		echo "Scanning $$service..."; \
		if ! (cd "$$service" && govulncheck -show verbose ./...); then status=1; fi; \
	done; \
	bash scripts/trivy_scan.sh all; trivy_status=$$?; \
	test "$$status" -eq 0 -a "$$trivy_status" -eq 0

# Run Trivy without the govulncheck scan.
trivy-security-check:
	bash scripts/trivy_scan.sh all

trivy-fs-security-check:
	bash scripts/trivy_scan.sh fs

trivy-image-security-check:
	bash scripts/trivy_scan.sh image

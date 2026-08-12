.PHONY: orchestrator pdf-service build typecheck test clean smoke smoke-real

orchestrator:
	cd orchestrator && go run ./cmd/server

pdf-service:
	cd pdf-service && npm run dev

build:
	cd orchestrator && go build ./...
	cd pdf-service && npm run build

typecheck:
	cd orchestrator && go vet ./...
	cd pdf-service && npm run typecheck

test:
	cd orchestrator && go test ./...
	cd pdf-service && npm test --if-present

clean:
	rm -rf orchestrator/data pdf-service/dist pdf-service/data

smoke:
	./scripts/smoke.sh

smoke-real:
	REAL=1 ./scripts/smoke.sh

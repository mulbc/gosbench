BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
UNIX_DATE := $(shell date -u +"%s")
VCS_REF := $(shell git rev-parse HEAD)

build:
	docker pull golang:alpine
	docker build --tag quay.io/mulbc/gosbench-server:$(VCS_REF) --build-arg "TYPE=server" --build-arg "BUILD_DATE=$(BUILD_DATE)" --build-arg "VCS_REF=$(VCS_REF)" .
	docker build --tag quay.io/mulbc/gosbench-worker:$(VCS_REF) --build-arg "TYPE=worker" --build-arg "BUILD_DATE=$(BUILD_DATE)" --build-arg "VCS_REF=$(VCS_REF)" .
debug-server:
	docker run --rm --name=gosbench-server -it quay.io/mulbc/gosbench-server:$(VCS_REF) sh
debug-worker:
	docker run --rm --name=gosbench-worker -it quay.io/mulbc/gosbench-worker:$(VCS_REF) sh
release:
	docker tag quay.io/mulbc/gosbench-server:$(VCS_REF) quay.io/mulbc/gosbench-server:latest
	docker tag quay.io/mulbc/gosbench-worker:$(VCS_REF) quay.io/mulbc/gosbench-worker:latest
	docker push quay.io/mulbc/gosbench-server:latest
	docker push quay.io/mulbc/gosbench-worker:latest
push-dev:
	docker build --tag quay.io/mulbc/gosbench-server:$(UNIX_DATE) --build-arg "TYPE=server" --build-arg "BUILD_DATE=$(BUILD_DATE)" --build-arg "VCS_REF=$(VCS_REF)" .
	docker build --tag quay.io/mulbc/gosbench-worker:$(UNIX_DATE) --build-arg "TYPE=worker" --build-arg "BUILD_DATE=$(BUILD_DATE)" --build-arg "VCS_REF=$(VCS_REF)" .
	docker push quay.io/mulbc/gosbench-server:$(UNIX_DATE)
	docker push quay.io/mulbc/gosbench-worker:$(UNIX_DATE)
test:
	go test -v `go list ./...`

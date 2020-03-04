BUILD_DATE = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
VCS_REF = $(shell git rev-parse HEAD)

build:
	docker build --tag quay.io/mulbc/goroom-server:dev --build-arg "TYPE=server" --build-arg "BUILD_DATE=$(BUILD_DATE)" --build-arg "VCS_REF=$(VCS_REF)" .
	docker build --tag quay.io/mulbc/goroom-worker:dev --build-arg "TYPE=worker" --build-arg "BUILD_DATE=$(BUILD_DATE)" --build-arg "VCS_REF=$(VCS_REF)" .
debug-server:
	docker run --rm --name=goroom-server -it quay.io/mulbc/goroom-server:dev sh
debug-worker:
	docker run --rm --name=goroom-worker -it quay.io/mulbc/goroom-worker:dev sh
release:
	docker tag quay.io/mulbc/goroom-server:dev quay.io/mulbc/goroom-server:latest
	docker tag quay.io/mulbc/goroom-worker:dev quay.io/mulbc/goroom-worker:latest
	docker push quay.io/mulbc/goroom-server:latest
	docker push quay.io/mulbc/goroom-worker:latest

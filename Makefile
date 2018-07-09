VERSION="0.1"
build:
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o webhook .
	docker build --no-cache -t jasonrichardsmith/mwc-example:${VERSION} .
	docker push jasonrichardsmith/mwc-example:${VERSION}
	rm -rf webhook

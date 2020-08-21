.PHONY: test clean

s3-object-router: *.go go.* cmd/s3-object-router/*.go
	cd cmd/s3-object-router && go build -o ../../s3-object-router .

test:
	go test ./...

clean:
	rm -rf s3-object-router dist/

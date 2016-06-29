all : git-pubsubhubbub

export GOPATH=$(CURDIR)/../../../..

git-pubsubhubbub : main.go pushhub/hub.go pushhub/store.go
	go build

testclient/testclient : testclient/testclient.go
	cd testclient && go build

check : git-pubsubhubbub testclient/testclient
	if [ -n "$$(gofmt -l .)" ]; then gofmt -d .; exit 1; fi
	go test

clean :
	rm -f git-pubsubhubbub testclient/testclient

deps :
	go get

install : git-pubsubhubbub
	install git-pubsubhubbub $(DESTDIR)$(prefix)/bin/git-pubsubhubbub

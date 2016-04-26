all : gitpubhook/gitpubhook

gitpubhook/gitpubhook : gitpubhook/main.go pushhub/hub.go pushhub/store.go
	cd gitpubhook && GOPATH=$(CURDIR)/../.. go build

testclient/testclient : testclient/testclient.go
	cd testclient && GOPATH=$(CURDIR)/../.. go build

check : gitpubhook/gitpubhook testclient/testclient
	true

clean :
	rm -f gitpubhook/gitpubhook testclient/testclient

install :
	install gitpubhook/gitpubhook $(DESTDIR)$(prefix)/bin/git-pubsubhubbub

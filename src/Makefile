UNAME := $(shell uname)
BINDIR := bin
EXE := scheduler

ifeq (${UNAME}, Darwin)
	BUILD_TIME=$(shell date +%Y-%m-%dT%H:%M:%S)
else
	BUILD_TIME=$(shell date --rfc-3339=seconds)
endif

all:
	mkdir -p ${BINDIR}
	CGO_ENABLED=0 go build -v -a -ldflags "-s -w -X \"main.BuildTime=${BUILD_TIME}\"" -o ${BINDIR}/${UNAME}/${EXE}

amd64:
	mkdir -p ${BINDIR}/amd64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -a -ldflags "-s -w -X \"main.BuildTime=${BUILD_TIME}\"" -o ${BINDIR}/amd64/${EXE}

docker:
	mkdir -p ${BINDIR}/docker
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -a -ldflags "-s -w -X \"main.BuildTime=${BUILD_TIME}\"" -o ${BINDIR}/docker/${EXE}

clean:
	go clean -cache -i -r
	rm -Rf ${BINDIR}

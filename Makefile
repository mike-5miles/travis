GOTOOLS = github.com/Masterminds/glide

all: get_vendor_deps install print_cybermiles_logo

get_vendor_deps: tools
	glide install
	@# cannot use ctx (type *"gopkg.in/urfave/cli.v1".Context) as type
	@# *"github.com/CyberMiles/travis/vendor/github.com/ethereum/go-ethereum/vendor/gopkg.in/urfave/cli.v1".Context ...
	@rm -rf vendor/github.com/ethereum/go-ethereum/vendor/gopkg.in/urfave

install:
	@echo "\n--> Installing the Travis TestNet\n"
	go install ./cmd/travis
	@echo "\n\nTravis, the TestNet for CyberMiles (CMT) has successfully installed!"

tools:
	@echo "--> Installing tools"
	go get $(GOTOOLS)
	@echo "--> Tools installed successfully"

build: get_vendor_deps
	go build -o build/travis ./cmd/travis

NAME := ywonline/travis
TAG := $(shell git rev-parse --short=8 HEAD)
IMAGE := ${NAME}:${TAG}
LATEST := ${NAME}:latest

docker_image:
	@docker build -t $(IMAGE) .
	@docker tag ${IMAGE} ${LATEST}

DEV := ${NAME}:develop
push_dev_image:
	@docker tag $(IMAGE) ${DEV}
	@docker push $(DEV)

push_image:
	@docker push $(LATEST)

print_cybermiles_logo:
	@echo "\n\n"
	@echo "    cmtt         tt                        cmt       tit ii  ll                 "
	@echo "  ttcmttt        tt                        tttt      ttt ii  ll                 "
	@echo " tt              tt                        cmtc     ittt     ll                 "
	@echo "it               tt                        mt t     titt ii  ll                 "
	@echo "tt      tt   cmt tt cmt     ,ttt    cm cmt mt tt   tt tt ii  ll    cmtt    cmtt "
	@echo "tt      itt   ti ttcmtttt  ttitttt  cmtcmt mt tt   tt tt ii  ll  ttttitt  tttiti"
	@echo "tt       tt  tt  tt    tt  tt   tt  tt     mt  ti  tt tt ii  ll  tt   tt  ti    "
	@echo "tt        t; tt  tt    tt  ttcmttt  tt     mt  tt it  tt ii  ll  ttcmttt  itttt "
	@echo "it,       tt t   tt    tt  ttcmtii  ti     mt   t tt  tt ii  ll  ttcmtii    tttt"
	@echo " cmt      tttt   tti   tt  tt       ti     mt   cmt   tt ii  ll  tt           tt"
	@echo "  ttcmttt  ttt   ttcmttt   ttcmttt  ti     mt   itt   tt ii  ll  tttttt   tcmttt"
	@echo "    iiii   tt    cmtcmt       iii   ii     mt    ii   ii ii  ll   ttii    iiii  "
	@echo "           ti                                                                   "
	@echo "          tt                                                                    "
	@echo "        ttt                                                                     "
	@echo "\n\n"
	@echo "Please visit the following URL for technical testnet instructions < https://github.com/CyberMiles/travis/blob/master/README.md >.\n"
	@echo "Visit our website < https://www.cybermiles.io/ >, to learn more about CyberMiles.\n"

# Use `=` instead of `:=` expanding variable lazely, not at beginning. Needed as
# elm files change during execution.
ELM_FILES = $(shell find src -iname *.elm)
# If JUNIT_DIR is set, the tests are executed with the JUnit reporter and the result is stored in the given directory.
JUNIT_DIR ?=
DOCKER              ?= docker
OPENAPI_GEN_IMAGE   ?= openapitools/openapi-generator-cli:v3.3.4
UI_BUILD_IMAGE      ?= quay.io/prometheus/golang-builder:1.26-base #  Derived from ../../.promu.yml.

.PHONY: all
all: src/Data build test

node_modules: package-lock.json
	npm ci

.PHONY: format
format: node_modules $(ELM_FILES)
	@echo ">> format front-end code"
	@npx elm-format --yes $(ELM_FILES)

.PHONY: review
review: node_modules
	@npx elm-review --fix

.PHONY: test
test: node_modules
	rm -rf elm-stuff/generated-code
	@npx elm-format $(ELM_FILES) --validate
	@npx elm-review
ifneq ($(JUNIT_DIR),)
	mkdir -p $(JUNIT_DIR)
	@npx elm-test --report=junit | tee $(JUNIT_DIR)/junit.xml
else
	@npx elm-test
endif

.PHONY: dev-server
dev-server:
	npx elm reactor

.PHONY: build
build: .build_stamp

.build_stamp: $(ELM_FILES) index.html vite.config.mjs node_modules
	@echo ">> building frontend"
	npm run build
	@touch .build_stamp

src/Data: ../../api/v2/openapi.yaml
	-rm -r src/Data
	set -e; \
	$(DOCKER) run \
	  --name openapi-gen \
	  -v ${PWD}/../..:/local \
	  $(OPENAPI_GEN_IMAGE) \
	  generate -i /local/api/v2/openapi.yaml -g elm -o /tmp/openapi-gen; \
	trap "$(DOCKER) rm openapi-gen" EXIT; \
	$(DOCKER) cp openapi-gen:/tmp/openapi-gen/src/Data src/Data; \
	$(DOCKER) cp openapi-gen:/tmp/openapi-gen/src/DateTime.elm src/DateTime.elm
	make format

.PHONY: container-build
container-build:
	set -e; \
	$(DOCKER) create \
	  --name ui-elm-builder \
	  --entrypoint sh \
	  $(UI_BUILD_IMAGE) \
	  -c "make test build"; \
	trap "$(DOCKER) rm ui-elm-builder" EXIT; \
	$(DOCKER) cp . ui-elm-builder:/app; \
	$(DOCKER) start --attach ui-elm-builder; \
	$(DOCKER) cp ui-elm-builder:/app/dist ./dist

.PHONY: clean
clean:
	- @rm -rf .build_stamp dist elm-stuff src/Data src/DateTime.elm openapi-* elm-*

# Use `=` instead of `:=` expanding variable lazely, not at beginning. Needed as
# elm files change during execution.
ELM_FILES = $(shell find src -iname *.elm)
# If JUNIT_DIR is set, the tests are executed with the JUnit reporter and the result is stored in the given directory.
JUNIT_DIR ?=
DOCKER              ?= docker
OPENAPI_GEN_IMAGE   ?= openapitools/openapi-generator-cli:v3.3.4
UI_BUILD_IMAGE      ?= quay.io/prometheus/golang-builder:1.26-base #  Derived from ../../.promu.yml.

.PHONY: all
all: script.js test

node_modules: package-lock.json
	npm ci

.PHONY: format
format: node_modules $(ELM_FILES)
	@echo ">> format front-end code"
	@npx elm-format --yes $(ELM_FILES)

.PHONY: review
review: src/Data node_modules
	@npx elm-review --fix

.PHONY: test
test: src/Data node_modules
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

# macOS requires mktemp template to be at the end of the filename,
# however --output flag for elm make must end in .js or .html.
script.js: export TEMPFILE := "$(shell mktemp ./elm-XXXXXXXXXX)"
script.js: export TEMPFILE_JS := "$(TEMPFILE).js"
script.js: src/Data node_modules format $(ELM_FILES)
	@echo ">> building script.js"
	@rm -rf elm-stuff
	@npx elm make src/Main.elm --optimize --output $(TEMPFILE_JS)
	@npx uglifyjs $(TEMPFILE_JS) --compress 'pure_funcs="F2,F3,F4,F5,F6,F7,F8,F9,A2,A3,A4,A5,A6,A7,A8,A9",pure_getters,keep_fargs=false,unsafe_comps,unsafe' --mangle --output $(@)
	@rm -rf $(TEMPFILE_JS)
	@rm -rf $(TEMPFILE)

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
	- @rm -rf script.js elm-stuff src/Data src/DateTime.elm openapi-* elm-*

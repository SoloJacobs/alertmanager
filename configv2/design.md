# configv2 design

Prototype for receiver configuration. Scope is the receiver/global half of the
config file. Route, inhibit rules and time intervals are untouched.

## What is wrong with v1

Three problems, all visible in `config/config.go`:

1. **Parse, don't validate.** Parse the configuration should result in an
   effective config, where invalid states cannot be represented.
2. **The global config knows every integration.** `GlobalConfig` has 38 fields
   named `SlackAPIURL`, `RocketchatTokenIDFile`, `MattermostWebhookURLFile` and
   so on, and `Config.UnmarshalYAML` carries a ~280 line block that walks each
   receiver and copies globals into it, per integration, by hand.
3. **Mutual exclusion is checked in two places.** `slack_api_url` vs
   `slack_app_token` is checked on the global struct, `api_url` vs `app_token`
   is checked again on the receiver struct, and the inheritance code has to
   re-derive which group the receiver already filled in so it does not mix
   half a webhook config with half a bot config.

## Goals

- Parse, do not validate. Parsing produces a type in which the invalid states
  cannot be written down.
- One integration, one package. Adding an integration touches no shared file
  other than the registration list.
- The global config contains no integration specific code or types.
- Every receiver key has a corresponding global key, with identical name and
  identical type.
- Receiver keys may be mutually exclusive. Global keys never are.

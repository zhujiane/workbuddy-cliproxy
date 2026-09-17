# Changelog

## [Unreleased]

- Fixed missing image input metadata for GLM-5.2, Kimi-K2.7, MiniMax-M3, Hy3, and DeepSeek-V4 Pro/Flash in both providers. Keep vision capability explicit per model; preserve text-only declarations for unconfirmed variants.

- Added international (`workbuddy`) GLM-5.3 support with `low`, `high`, and `max` thinking levels; default to `high` while preserving explicit reasoning controls. Advertises the default 300,000-token context (supports expansion to 1,000,000).

## [0.2.1] - 2026-09-15

### Fixed

- Declared `deepseek-v4.1-flash` as accepting `text` and `image` input, and preserved multimodal message parts when forwarding requests.
- Added DeepSeek thinking levels `low`, `high`, and `max`. When no thinking setting is supplied, the plugin defaults DeepSeek to `reasoning_effort=high`; explicit `reasoning_effort` and `thinking` values remain unchanged.
- Added the required leading `system` message for CodeBuddy requests whose first message is `user`, and converted a leading `developer` message to `system`. This fixes CodeBuddy business error `11128` (`first message is not system prompt`).
- Preserved upstream HTTP status codes and stream errors so CPA can report the actual cause instead of masking it as `503 auth_unavailable`.
- Added bounded retries for connection errors and upstream `502`/`503`/`504` before a response starts. Streaming responses are never replayed after output begins.
- Removed the whole-response timeout from chat streams and surfaced scanner/read failures instead of treating an interrupted stream as a successful completion.
- Preserved model capability metadata in the CPA OpenAI model list, including input/output modalities, supported parameters, thinking levels, context length, and output limits.

### Compatibility

- `workbuddy` and `workbuddy-cn` are released as version `0.2.1`.
- DeepSeek defaults to `reasoning_effort=high`; clients can select `low`, `high`, or `max` explicitly.

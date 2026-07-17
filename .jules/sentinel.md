## 2024-05-15 - [SSRF Bypass in Webhooks]
**Vulnerability:** The webhook validation function had empty code blocks for filtering localhost and private IPs, effectively allowing Server-Side Request Forgery (SSRF) against internal services.
**Learning:** `net.ParseIP("localhost")` returns `nil`. Thus, explicit string checks for "localhost" are necessary alongside IP checks to properly block SSRF. Empty if-statements left as placeholders create severe vulnerabilities.
**Prevention:** Ensure all validation logic blocks explicitly return rejection states (`return false`) rather than leaving empty placeholders. Always handle the `localhost` edge case when validating IP addresses against private ranges.

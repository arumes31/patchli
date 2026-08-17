## 2024-07-30 - TOCTOU DNS Rebinding SSRF
**Vulnerability:** HTTP clients hitting external webhook URLs were susceptible to DNS Rebinding SSRF. A URL could resolve to a benign IP during validation (`isValidURL`), but point to a local/private IP when the actual request was made, bypassing initial checks.
**Learning:** Checking hostnames or IPs before making the request is insufficient due to Time-of-Check to Time-of-Use (TOCTOU) issues with DNS resolution.
**Prevention:** Always implement a custom `DialContext` in the `http.Client`'s `Transport`. Resolve the IP explicitly inside the dialer, validate the resolved IP against private/loopback ranges, and then dial that *exact* resolved IP directly to guarantee the validated IP is the one actually connected to.

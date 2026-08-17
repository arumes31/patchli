## 2025-02-23 - SSRF and DNS Rebinding Prevention in Webhooks
**Vulnerability:** Webhook dispatchers use standard `http.Client` which resolves DNS at request time, allowing attackers to bypass URL validation by serving a safe IP during validation and a private/loopback IP during the actual request (DNS Rebinding/TOCTOU SSRF).
**Learning:** Boolean string-based validation (`isValidURL`) is insufficient against dynamic DNS resolutions.
**Prevention:** Implement a custom `DialContext` on the `http.Transport` that resolves the IP, validates it, and directly dials the resolved IP to guarantee consistency between validation and connection.

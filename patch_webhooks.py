with open('server/internal/webhooks/webhooks.go', 'r') as f:
    content = f.read()

content = content.replace('client.Post(webhookURL, "application/json", bytes.NewBuffer(data)) // #nosec G107 -- webhookURL is validated by isValidURL', 'client.Post(webhookURL, "application/json", bytes.NewBuffer(data)) // #nosec G107 G704 -- webhookURL is validated by isValidURL')

with open('server/internal/webhooks/webhooks.go', 'w') as f:
    f.write(content)

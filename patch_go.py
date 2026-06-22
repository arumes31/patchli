with open('server/go.mod', 'r') as f:
    content = f.read()

content = content.replace('go 1.24', 'go 1.25.0')

with open('server/go.mod', 'w') as f:
    f.write(content)

with open('agent/go.mod', 'r') as f:
    content = f.read()

content = content.replace('go 1.24', 'go 1.26.3')

with open('agent/go.mod', 'w') as f:
    f.write(content)

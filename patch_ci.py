import re

with open('.github/workflows/ci.yml', 'r') as f:
    content = f.read()

content = content.replace('''      - name: golangci-lint (Server)
        uses: golangci/golangci-lint-action@v4
        with:
          version: latest
          working-directory: server
          args: --timeout=5m''', '''      - name: golangci-lint (Server)
        uses: golangci/golangci-lint-action@v4
        with:
          version: v1.64.6
          working-directory: server
          args: --timeout=5m''')


content = content.replace('''      - name: golangci-lint (Agent)
        uses: golangci/golangci-lint-action@v4
        with:
          version: latest
          working-directory: agent
          args: --timeout=5m''', '''      - name: golangci-lint (Agent)
        uses: golangci/golangci-lint-action@v4
        with:
          version: v1.64.6
          working-directory: agent
          args: --timeout=5m''')

with open('.github/workflows/ci.yml', 'w') as f:
    f.write(content)

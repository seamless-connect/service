# Stateless Hosting (Go)

This is a Go port of the simple Python bottle application implementing "stateless hosting" using Domain Connect.

The page rendered for a domain simply displays the host (domain) name and simple message configurable by the user. The message is cleverly stored in DNS, allowing for the "stateless" claim.

DNS for a domain hosted on this platform requires two records. One for an A Record's IP address. The other for a TXT record containing the message.

The application asks for a domain name and message. Once input, the application runs Domain Connect to first discover the identity of the DNS Provider, and then to configure the two values in DNS.

## Running

### Locally

```bash
go mod init exampleservice-golang  # if needed
go mod tidy
go run .
```

The server will start on port 8080.

### With Docker

```bash
docker-compose up --build
```

### Environment Variables

- `APP_IP` - IP address of the server (default: `132.148.166.208`)
- `APP_DOMAIN` - hostname of the application (default: `exampleservice.domainconnect.org`)
- `APP_PROTOCOL` - protocol (`http` or `https`, default: `https`)

## Templates

The application uses two templates. Both are similar, but one adds an additional CNAME and uses signatures:

- [Template 1](https://github.com/Domain-Connect/Templates/blob/master/exampleservice.domainconnect.org.template1.json)
- [Template 2](https://github.com/Domain-Connect/Templates/blob/master/exampleservice.domainconnect.org.template2.json)

## Dependencies

- Go 1.22+
- Standard library (`net/http`, `html/template`, `crypto/rsa`, etc.)

No external dependencies are required.

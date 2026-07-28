package main

import "os"

var (
	// IP is the IP address of the server running this code
	IP = getEnv("APP_IP", "132.148.166.208")

	// HostingWebsite is the host name of our application
	HostingWebsite = getEnv("APP_DOMAIN", "exampleservice.domainconnect.org")

	// DynamicDNSWebsite is the host name of the dynamic dns app
	DynamicDNSWebsite = "dynamicdns.domainconnect.org"

	// Protocol for the app
	Protocol = getEnv("APP_PROTOCOL", "https")

	// PrivKey is the private key used to generate signatures
	PrivKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA18SgvpmeasN4BHkkv0SBjAzIc4grYLjiAXRtNiBUiGUDMeTzQrKTsWvy9NuxU1dIHCZy9o1CrKNg5EzLIZLNyMfI6qiXnM+HMd4byp97zs/3D39Q8iR5poubQcRaGozWx8yQpG0OcVdmEVcTfyR/XSEWC5u16EBNvRnNAOAvZYUdWqVyQvXsjnxQot8KcK0QP8iHpoL/1dbdRy2opRPQ2FdZpovUgknybq/6FkeDtW7uCQ6Mvu4QxcUa3+WP9nYHKtgWip/eFxpeb+qLvcLHf1h0JXtxLVdyy6OLk3f2JRYUX2ZZVDvG3biTpeJz6iRzjGg6MfGxXZHjI8weDjXrJwIDAQABAoIBAGiPedJDwXg9d1i7mCo0OY8z1qPeFh9OGP/Zet8i9bQPN2gjahslTNtK07cDC8C2aFRz8Xw3Ylsk5VxdNobzjFPDNUM6JhawnvR0jQU5GhdTwoc5DHH7aRRjTP6m938sRx0VrfZwfvJAB09Z4jHX7vyjfvprH9EH8GQ2L5lACtfnsSASVJB77H1vtgxTnum74CSqIck1MCjPD/TVUtYfMJwkUQWcbk79N4nvnEoagqsDrvw4okU2OYMWucQjyxfWTU4NGlsDScRbdDAb8sLr3DpMfXM8vpZJ3Ed6gfw14hEJym8XoHwDHmjGmgYH9iG6MODxuO5TLRmRR6b+jcUV/2kCgYEA4WGsDUO/NIXIqtDm5lTi5qeFl0sGKIgRLGuCrvjLF0Fq5Yx28wuow3OhZ3rbjlmhf9nUt24nUUY67plv2pi+vx3kVdbcNfk+Wkc0wfx8+U91qaTplMRhNjrnq/Kp9E7xtnzZRInpUG1Ha5ozTYobVvklUvjodFlF2c16Zz2X2AMCgYEA9RSeZm7oMyJbe985SScXruwt5ZXlUBoBLDZAeMloPpaqknFmSVSNgtniywztF8HppJQyiMvmUOUL2tKnuShXwsvTkCTBC/vNGXutiPS8O2yqeQ8dHoHuKcoMFwgajrbPrVkuFtUkjbQJ/TKoZtrxUdCryDZ/AHmRtiHh9E4NUQ0CgYAE7ngvSh4y7gJ4Cl4jCBR26492wgN+e4u0px2S6oq3FY1bPHmV09l7fVo4w21ubfOksoV/BgACPUEo216hL9psoCDQ6ASlgbCllQ1IeVfatKxka+FYift+jkdnccXaPKf5UD4Iy+O5CMsZRaR9u9nhS05PxHaBpTpsC5z0CVr7NQKBgQCsBTzpSQ9SVNtBpvzei8Hj1YKhkwTRpG8OSUYXgcbZp4cyIsZY0jBBmA3H19rSwhjsm9icjAGs5hfcD+AJ5nczEz37/tBBSQw8xsKXTrCQRUWikyktMKWqT1cNE3MQmOBMHDxtak2t6KDaR6RMDYE0m/L3JMkf3DSaUk323JIcQQKBgD6lHhw79Cenpezzf0566uWE1QF6Sv3kWk6Gkzo2jUGmjo2tG1v2Nj82DvcTuqvfUKSr2wTKINxnKGyYXGto0BykdxeFbR04cNcBB46zUjasro2ZCvIoAHCpohNBI2dL6dI+RI3jC/KY3jPNI0toaOTWkeAvJ7w09G2ttlv8qLNV
-----END RSA PRIVATE KEY-----`

	// Provider is the Domain Connect provider
	Provider = "exampleservice.domainconnect.org"

	// Template1 is the first template
	Template1 = "template1"

	// Template2 is the second template
	Template2 = "template2"

	// OAuthSecrets holds secrets per provider that support OAuth
	OAuthSecrets = map[string]string{
		"GoDaddy":       "DomainConnectGeheimnisSecretString",
		"Secure Server": "DomainConnectGeheimnisSecretString",
		"1and1":         "cd$;CVZRj#B8C@o3o8E4v-*k2H7S%)",
	}

	// OAuthAPIURLs holds API URLs per provider
	OAuthAPIURLs = map[string]string{
		"GoDaddy":       "https://domainconnect.api.godaddy.com",
		"Secure Server": "https://domainconnect.api.secureserver.net",
		"1and1":         "https://api.domainconnect.1and1.com",
	}
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

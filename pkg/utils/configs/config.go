package configs

func ConnectJSON() string {
	connectJSON := `
{
    "scheme": "amqps",
    "host": "skupper-router",
    "port": "5671",
    "tls": {
        "ca": "/etc/messaging/ca.crt",
        "cert": "/etc/messaging/tls.crt",
        "key": "/etc/messaging/tls.key",
        "verify": true
    }
}
`
	return connectJSON
}

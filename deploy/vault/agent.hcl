pid_file = "/tmp/vault-agent.pid"

vault {
  address = "http://vault:8200"
}

auto_auth {
  method {
    type = "token_file"
    config = {
      token_file_path = "/vault/config/dev-token"
    }
  }
}

template {
  source      = "/vault/config/templates/datasafe.env.tpl"
  destination = "/rendered/datasafe.env"
  perms       = 0600
  command     = ["sh", "-c", "test -s /rendered/datasafe.env"]
}
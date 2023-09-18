variable "target_hosts" {
  type        = list(string)
  default     = null
  description = "The addresses of the target hosts to connect to. This can be an IP address or a DNS name."
}

variable "target_host_authn_type" {
  type        = string
  default     = "ssh"
  description = "The type of authentication to use for the target host. Valid values are `ssh` and `winrm`."
}

variable "target_host_authn_user" {
  type        = string
  default     = "root"
  description = "The user to use for authenticating to the target host."
}

variable "target_host_authn_secret" {
  type        = string
  sensitive   = true
  description = "The secret to use for authenticating to the target host. This can be a password or a private key."
}

variable "target_host_insecure" {
  type        = bool
  default     = true
  description = "Whether to skip TLS verification when connecting to the target host."
}

resource "courier_target" "example" {
  count = length(var.target_hosts)

  host = {
    address = var.target_hosts[count.index]
    authn   = {
      type   = var.target_host_authn_type
      user   = var.target_host_authn_user
      secret = var.target_host_authn_secret
    }
    insecure = var.target_host_insecure
  }

  timeouts = {
    create = "5m"
    update = "5m"
  }
}

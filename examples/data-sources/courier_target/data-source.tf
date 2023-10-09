variable "target_addresses" {
  type        = list(string)
  description = "The addresses of the target hosts to connect to. Item can be a IP[:Port] address or a DNS name."

  validation {
    condition = alltrue([
      for address in var.target_addresses :
      anytrue([
        can(regex("^(([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])\\.){3}([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])$", split(":", address)[0])),
        can(regex("^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\\-]*[a-zA-Z0-9])\\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\\-]*[A-Za-z0-9])$", split(":", address)[0]))
      ])
    ])
    error_message = "Invalid target address, must be a valid IP or DNS name."
  }
}

variable "target_authn_type" {
  type        = string
  description = "The type of authentication to use for the target host, either `ssh` or `winrm`."
  default     = "ssh"

  validation {
    condition     = contains(["ssh", "winrm"], var.target_authn_type)
    error_message = "Invalid target authentication type, must be one of `ssh` or `winrm`."
  }
}

variable "target_authn_user" {
  type        = string
  description = "The user to use for authenticating to the target host."
  default     = "root"

  validation {
    condition     = length(var.target_authn_user) > 0
    error_message = "Invalid target authentication user, must be at least 1 character long."
  }
}

variable "target_authn_secret" {
  type        = string
  description = "The secret to use for authenticating to the target host. This can be a password or a private key."
  sensitive   = true

  validation {
    condition     = length(var.target_authn_secret) > 0
    error_message = "Invalid target authentication secret, must be at least 1 character long."
  }
}

variable "target_insecure" {
  type        = bool
  description = "Whether to skip TLS verification when connecting to the target host."
  default     = true
}

variable "target_proxies" {
  type = list(object({
    address      = string
    insecure     = bool
    authn_type   = string
    authn_user   = string
    authn_secret = string
  }))
  description = "The proxies to use when connecting to the target host. Item can be a bastion host or connection proxy."
  default     = []

  validation {
    condition = length(var.target_proxies) == 0 || alltrue([
      for proxy in var.target_proxies :
      alltrue([
        can(regex("^(([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])\\.){3}([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])$", split(":", proxy.address)[0])),
        can(regex("^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\\-]*[a-zA-Z0-9])\\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\\-]*[A-Za-z0-9])$", split(":", proxy.address)[0]))
      ])
    ])
    error_message = "Invalid proxy address, must be a valid IP or DNS name."
  }

  validation {
    condition = length(var.target_proxies) == 0 || alltrue([
      for proxy in var.target_proxies : contains(["ssh", "proxy"], proxy.authn_type)
    ])
    error_message = "Invalid proxy authentication type, must be one of `ssh` or `proxy`."
  }
}

data "courier_target" "example" {
  count = length(var.target_addresses)

  host = {
    address = var.target_addresses[count.index]
    authn   = {
      type   = var.target_authn_type
      user   = var.target_authn_user
      secret = var.target_authn_secret
    }
    insecure = var.target_insecure
    proxies  = length(var.target_proxies) > 0 ? [
      for proxy in var.target_proxies : {
        address = proxy.address
        authn   = {
          type   = proxy.authn_type
          user   = proxy.authn_user
          secret = proxy.authn_secret
        }
        insecure = proxy.insecure
      }
    ] : null
  }

  timeouts = {
    read = "10m"
  }
}

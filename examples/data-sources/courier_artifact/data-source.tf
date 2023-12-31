variable "artifact_refer_uri" {
  type        = string
  description = "The URI of the artifact to be pulled."
  default     = "https://tomcat.apache.org/tomcat-7.0-doc/appdev/sample/sample.war"
}

variable "artifact_refer_authn_type" {
  type        = string
  description = "The type of authentication to be used for pulling the artifact."
  default     = ""

  validation {
    condition     = length(var.artifact_refer_authn_type) == 0 || contains(["basic", "bearer"], var.artifact_refer_authn_type)
    error_message = "Invalid artifact authentication type, must be one of `basic` or `bearer`."
  }
}

variable "artifact_refer_authn_user" {
  type        = string
  description = "The username of the authentication to be used for pulling the artifact."
  default     = ""
}

variable "artifact_refer_authn_secret" {
  type        = string
  description = "The secret of the authentication to be used for pulling the artifact, either password or token."
  default     = ""
}

variable "artifact_refer_insecure" {
  type        = bool
  description = "Whether to skip TLS verification when pulling the artifact."
  default     = true
}

variable "artifact_command" {
  type        = string
  description = "The command to start the artifact."
  default     = ""
}

variable "artifact_ports" {
  type        = list(number)
  description = "The ports to be exposed by the artifact."
  default     = null
}

variable "artifact_envs" {
  type        = map(string)
  description = "The environment variables to be set for the artifact."
  default     = null
}

variable "artifact_volumes" {
  type        = list(string)
  description = "The volumes to be mounted for the artifact."
  default     = null
}

data "courier_artifact" "example" {
  refer = {
    uri   = var.artifact_refer_uri
    authn = length(var.artifact_refer_authn_type) > 0 ? {
      type   = var.artifact_refer_authn_type
      user   = var.artifact_refer_authn_user
      secret = var.artifact_refer_authn_secret
    } : null
    insecure = var.artifact_refer_insecure
  }

  command = var.artifact_command
  ports   = var.artifact_ports
  envs    = var.artifact_envs
  volumes = var.artifact_volumes

  timeouts = {
    read = "10m"
  }
}
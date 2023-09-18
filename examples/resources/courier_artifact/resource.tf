variable "artifact_refer_uri" {
  type        = string
  default     = "https://tomcat.apache.org/tomcat-7.0-doc/appdev/sample/sample.war"
  description = "The URI of the artifact to be deployed."
}

variable "artifact_refer_authn_type" {
  type        = string
  default     = null
  description = "The type of authentication to be used for pulling the artifact."
}

variable "artifact_refer_authn_user" {
  type        = string
  default     = ""
  description = "The username of the authentication to be used for pulling the artifact."
}

variable "artifact_refer_authn_secret" {
  type        = string
  default     = ""
  description = "The secret of the authentication to be used for pulling the artifact, either password or token."
}

variable "artifact_refer_insecure" {
  type        = bool
  default     = true
  description = "Whether to skip TLS verification when pulling the artifact."
}

variable "artifact_runtime" {
  type        = string
  default     = "tomcat"
  description = "The runtime of the artifact to be deployed."
}

variable "artifact_command" {
  type        = string
  default     = ""
  description = "The command to start the artifact."
}

variable "artifact_ports" {
  type        = list(number)
  default     = [443, 80]
  description = "The ports to be exposed by the artifact."
}

variable "artifact_envs" {
  type        = map(string)
  default     = null
  description = "The environment variables to be set for the artifact."
}

variable "artifact_volumes" {
  type        = list(string)
  default     = null
  description = "The volumes to be mounted for the artifact."
}

resource "courier_artifact" "example" {
  refer = {
    uri   = var.artifact_refer_uri
    authn = var.artifact_refer_authn_type ? {
      type   = var.artifact_refer_authn_type
      user   = var.artifact_refer_authn_user
      secret = var.artifact_refer_authn_secret
    } : null
    insecure = true
  }

  runtime = var.artifact_runtime
  command = var.artifact_command
  ports   = var.artifact_ports
  envs    = var.artifact_envs
  volumes = var.artifact_volumes

  timeouts = {
    create = "5m"
    update = "5m"
  }
}
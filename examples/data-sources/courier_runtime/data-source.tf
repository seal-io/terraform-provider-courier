variable "runtime_source" {
  type        = string
  description = "The source of the runtime."
  default     = "https://github.com/seal-io/terraform-provider-courier//pkg/runtime/source_builtin?ref=v0.0.4"
}

variable "runtime_class" {
  type        = string
  description = "The class of the runtime to be used."
  default     = "tomcat"

  validation {
    condition     = contains(["tomcat", "openjdk", "docker"], var.runtime_class)
    error_message = "Invalid runtime runtime, must be one of `tomcat`, `openjdk` or `docker`."
  }
}

data "courier_runtime" "example" {
  source = var.runtime_source
  class  = var.runtime_class

  timeouts = {
    read = "5m"
  }
}

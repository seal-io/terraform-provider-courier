variable "deployment_strategy" {
  type        = string
  description = "The deployment strategy to use, either `recreate` or `rolling`."
  default     = "recreate"

  validation {
    condition     = contains(["recreate", "rolling"], var.deployment_strategy)
    error_message = "Invalid deployment strategy, must be one of `recreate` or `rolling`."
  }
}

variable "deployment_strategy_rolling_max_surge" {
  type        = number
  description = "The maximum percent of targets to deploy at once during rolling."
  default     = 0.3

  validation {
    condition     = var.deployment_strategy_rolling_max_surge >= 0.1 && var.deployment_strategy_rolling_max_surge <= 1
    error_message = "Invalid deployment rolling maximum surge, must be between 0.1 and 1.0."
  }
}

variable "deployment_progress_timeout" {
  type        = string
  description = "The timeout for deployment progress."
  default     = "5m"
}

resource "courier_deployment" "example" {
  artifact = {
    id    = "..."
    refer = {
      uri = "..."
    }
    runtime = "..."
  }

  targets = [
    {
      id   = "..."
      host = {
        address = "..."
        authn   = {
          type   = "..."
          user   = "..."
          secret = "..."
        }
      }
    }
  ]

  strategy = {
    type    = var.deployment_strategy
    rolling = var.deployment_strategy == "rolling" ? {
      max_surge = var.deployment_strategy_rolling_max_surge
    } : null
  }

  timeouts = {
    create = var.deployment_progress_timeout
    update = var.deployment_progress_timeout
    delete = var.deployment_progress_timeout
  }
}
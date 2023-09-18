resource "courier_deployment" "example" {
  artifact = {
    id    = "..."
    refer = {
      uri = "..."
    }
    runtime = "tomcat"
  }

  targets = [
    {
      id   = "..."
      host = {
        address = "..."
        authn   = {
          type   = "ssh"
          user   = "root"
          secret = "..."
        }
        insecure = true
        proxies  = [
          {
            address = "..."
            authn   = {
              type   = "ssh"
              user   = "root"
              secret = "..."
            }
          }
        ]
      }
    }
  ]

  strategy = {
    type    = "rolling"
    rolling = {
      max_surge = 0.3
    }
  }

  timeouts = {
    create = "5m"
    update = "5m"
    delete = "5m"
  }
}
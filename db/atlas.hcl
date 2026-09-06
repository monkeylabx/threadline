variable "url" {
  type    = string
  default = getenv("THREADLINE_MIGRATION_URL")
}

env "threadline" {
  url = var.url

  migration {
    dir = "file://migrations?format=golang-migrate"
  }
}

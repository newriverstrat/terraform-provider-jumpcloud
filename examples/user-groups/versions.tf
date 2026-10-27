terraform {
  required_providers {
    jumpcloud = {
      source = "newriverstrat/jumpcloud"
    }
  }
}

provider "jumpcloud" {
  api_key = "MY_API_KEY"
}

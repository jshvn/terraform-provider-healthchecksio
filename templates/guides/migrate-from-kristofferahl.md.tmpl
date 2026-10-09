---
page_title: "Move from kristofferahl/healthchecksio"
description: |-
  Move the checks from the kristofferahl/healthchecksio provider to this provider.
---

# Move from kristofferahl/healthchecksio

This provider and `kristofferahl/healthchecksio` have the same resource type name
(`healthchecksio_check`) and the same environment variables (`HEALTHCHECKSIO_API_KEY` and
`HEALTHCHECKSIO_API_URL`). The schemas are different:

| kristofferahl/healthchecksio | This provider |
|---|---|
| `timezone` | `tz` |
| `tags`, a list | `tags`, a set |
| `channels`, a list | `channels`, a set of lowercase UUIDs |
| The id is the UUID | The id is the unique_key. `uuid` is a sensitive attribute |
| API v1 | API v3. An `api_url` must end in `/api/v3/` |

The state of the old provider does not contain the attributes of this provider. Thus, do
not move the state. Remove the checks from the state, and import them again. The checks
stay on the server.

## Procedure

1. Remove each check from the state. The check stays on the server:

   ```shell
   terraform state rm healthchecksio_check.backup
   ```

2. Change the source of the provider, and get the new provider:

   ```terraform
   terraform {
     required_providers {
       healthchecksio = {
         source  = "jshvn/healthchecksio"
         version = "~> 1.0"
       }
     }
   }
   ```

   ```shell
   terraform init -upgrade
   ```

3. Change `timezone` to `tz` in each resource.
4. If an `api_url` ends in `/api/v1/`, change it to `/api/v3/`.
5. Add an `import` block for each check. The `id` is the unique_key of the check. The
   `healthchecksio_checks` data source gives the unique_key of each check, in `id`:

   ```terraform
   import {
     to = healthchecksio_check.backup
     id = "a6c7b0a8a66bed0df66abfdab3c77736861703ee"
   }
   ```

6. Run `terraform plan`. If the plan shows a change other than an import, make the
   configuration agree with the check.

With Terraform 1.14 or later, `terraform query` can write the import blocks and the
resource blocks. See the `healthchecksio_check` list resource.

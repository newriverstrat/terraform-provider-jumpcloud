# Releasing

This is a fork of [cheelim1/terraform-provider-jumpcloud](https://github.com/cheelim1/terraform-provider-jumpcloud),
published to the Terraform Registry as **`newriverstrat/jumpcloud`**.

## Branch model

| Branch | Purpose |
|---|---|
| `master` | Mirrors upstream, untouched. Keeps our diff against upstream clean so we can keep pulling their changes and sending them PRs. |
| `nrs-master` | Our line. Everything we build and release comes from here. |

Work lands on `nrs-master` via PR. Anything generally useful should **also** go upstream as a PR
from a branch based on `master`, so the two don't drift further than necessary.

To pick up upstream changes:

```bash
git fetch upstream                 # git@github.com:cheelim1/terraform-provider-jumpcloud.git
git checkout master && git merge --ff-only upstream/master && git push origin master
git checkout nrs-master && git merge master      # resolve, PR if non-trivial
```

## One-time setup

Only needed once per repo. All of it is outside the code.

### 1. Signing key

The Terraform Registry will not accept a release whose `SHA256SUMS` isn't GPG-signed.

```bash
gpg --full-generate-key          # RSA 4096; use a passphrase
gpg --list-secret-keys --keyid-format=long      # note the key ID
gpg --armor --export-secret-keys <KEY_ID>       # -> GPG_PRIVATE_KEY secret
gpg --armor --export <KEY_ID>                   # -> upload to the registry (step 3)
```

### 2. Repository secrets

Settings → Secrets and variables → Actions:

| Secret | Value |
|---|---|
| `GPG_PRIVATE_KEY` | ASCII-armored **private** key from step 1 |
| `PASSPHRASE` | That key's passphrase |

(`GITHUB_TOKEN` is provided automatically — nothing to add.)

### 3. Registry

1. Sign in to [registry.terraform.io](https://registry.terraform.io) with the GitHub org account.
2. User settings → **Signing Keys** → add the ASCII-armored **public** key from step 1.
3. **Publish** → **Provider** → select `terraform-provider-jumpcloud`.

The repo must stay public and keep the `terraform-provider-` name prefix — the registry
requires both.

## Versioning

Our releases use a **`v1.x.x`** series. Upstream is on `v0.3.x`, so this can't be confused
with theirs even though the two live in different registry namespaces.

> **Do not push upstream tags to `origin`.** The release workflow fires on any `v*` tag. If you
> run `git fetch upstream --tags && git push origin --tags`, you will publish upstream's code
> under our namespace. Fetch upstream tags only if you need them, and never bulk-push tags.

## Cutting a release

```bash
git checkout nrs-master && git pull
git tag v1.0.0
git push origin v1.0.0
```

The `release` workflow then builds every platform, signs the checksums, and creates the GitHub
release. The registry ingests it within a few minutes. If the workflow fails after the tag is
already pushed, re-run it from the Actions tab (`workflow_dispatch`) rather than deleting and
re-pushing the tag.

Verify at `https://registry.terraform.io/providers/newriverstrat/jumpcloud/latest`.

## Consuming it

```hcl
terraform {
  required_providers {
    jumpcloud = {
      source  = "newriverstrat/jumpcloud"
      version = "1.0.0"
    }
  }
}
```

Note that `newriverstrat/jumpcloud` and `cheelim1/jumpcloud` are *different providers* to
Terraform, not two versions of one. Moving an existing state between them is:

```bash
terraform state replace-provider cheelim1/jumpcloud newriverstrat/jumpcloud
```

which is also how we'd move **back** to upstream if our changes land there and the fork stops
earning its keep.

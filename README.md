# oci-nuke

Remove all resources from an Oracle Cloud Infrastructure (OCI) compartment.

> **This is potentially very destructive! Use at your own risk!**
> Always run without `--no-dry-run` first and review what would be removed before
> actually destroying anything.

## Overview

`oci-nuke` exists to support fast, repeatable testing of
[entigo-infralib](https://github.com/entigolabs/entigo-infralib)'s Oracle Cloud modules:
provision a compartment from scratch, test, nuke it clean, repeat. There is no
established "oci-nuke" tool comparable to
[aws-nuke](https://github.com/ekristen/aws-nuke) or
[gcp-nuke](https://github.com/ekristen/gcp-nuke) for OCI, so this project fills that
gap using the same underlying engine those two tools are built on:
[ekristen/libnuke](https://github.com/ekristen/libnuke).

libnuke provides the generic pieces (dependency-aware removal ordering, retry-until-no-
progress scanning, filtering, config-driven blocklists) so this repo only needs to
implement OCI-specific resource listers and removers. See libnuke's own documentation
for how the engine and config format work in more depth.

Resource coverage today is intentionally narrow - only what entigo-infralib's Oracle
bootstrap (via `entigo-infralib-agent`) and its `oracle/vpc` terraform module create:

* Networking: VCN, Subnet, Route Table (including a VCN's default route table),
  Internet Gateway, NAT Gateway, Service Gateway
* Object Storage: Bucket (including all object versions, not just current ones)
* Logging: Log Group, Log
* Compute: Container Instance
* DevOps: Project, Deploy Pipeline, Build Pipeline
* Notifications: ONS Topic
* IAM: Dynamic Group, Policy, Customer Secret Key

As entigo-infralib's Oracle module set grows (OKE, Vault, etc.), this list needs to
grow with it. Contributions and issues welcome.

## Requirements

* Go 1.26+
* OCI credentials - either an API-key/session-token profile in `~/.oci/config`
  (respecting `OCI_CONFIG_FILE`), or resource principal when run inside an OCI
  Container Instance. Same credential resolution as the `oci` CLI and
  `entigo-infralib-agent`.

## Building

```shell
go build -o bin/oci-nuke .
```

## Docker

```shell
docker build -t entigolabs/oci-nuke .
```

## Configuration

Every compartment you intend to nuke must be declared explicitly in the config file -
this is a safety net, not a discovery mechanism. Copy the example and fill in your
compartment OCID:

```shell
cp config.yaml.example config.yaml
```

```yaml
regions:
  - eu-frankfurt-1

accounts:
  ocid1.compartment.oc1..your-compartment-ocid: {}
```

`config.yaml` is gitignored since it typically contains a real compartment OCID for
your environment; `config.yaml.example` is the tracked template.

## Usage

```shell
./bin/oci-nuke run \
  --compartment-id ocid1.compartment.oc1..your-compartment-ocid \
  --region eu-frankfurt-1 \
  --prefix myprefix
```

Without `--no-dry-run`, this only lists what would be removed. Add `--no-dry-run` to
actually delete resources, and `--no-prompt` to skip the interactive confirmation
(useful in CI, dangerous everywhere else).

### Flags / environment variables

| Flag              | Env var                    | Description                                                                                                                 |
|--------------------|-----------------------------|-------------------------------------------------------------------------------------------------------------------------------|
| `--compartment-id` | `OCI_NUKE_COMPARTMENT_ID`   | OCID of the compartment to nuke.                                                                                             |
| `--region`         | `OCI_REGION`                | OCI region the compartment's resources live in.                                                                             |
| `--prefix`         | `OCI_NUKE_PREFIX`           | Deployment prefix, used to find and filter this deployment's own tenancy-wide resources (dynamic groups, policies, secret keys) out of a namespace shared with everyone else in the tenancy. |
| `--config`         | -                            | Path to the config file (default `config.yaml`).                                                                            |
| `--no-dry-run`     | -                            | Actually remove resources. Without it, only a dry-run listing is printed.                                                   |
| `--no-prompt`      | -                            | Skip the interactive confirmation prompt.                                                                                   |
| `--include` / `--exclude` | -                      | Scope a run to specific resource types (e.g. `--include OCIBucket`).                                                         |

## Why not just fork aws-nuke/gcp-nuke?

Both are architecturally AWS/GCP-specific at the CLI and account-resolution layer.
Building on `libnuke` directly gives the same dependency/retry engine those tools use
without carrying code for a cloud we don't need.

## License

MIT, consistent with [libnuke](https://github.com/ekristen/libnuke),
[aws-nuke](https://github.com/ekristen/aws-nuke), and
[gcp-nuke](https://github.com/ekristen/gcp-nuke), the projects this tool is built on
and modeled after.

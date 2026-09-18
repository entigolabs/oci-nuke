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

Note: `go.mod` replaces libnuke with the patched copy in `third_party/libnuke` (one-line
fix in `pkg/nuke/nuke.go`). Upstream v1.3.0 marks every item of a resource type that
declares `DependsOn` as `ItemStateNewDependency` at scan time - even when nothing it
depends on still exists - while `Run()`'s "No resource to delete" early-exit counts only
`ItemStateNew`. The tail end of a partially-completed nuke (subnets, route tables, NSGs,
gateways - all dependency-declaring types here) therefore falsely reported nothing left
to delete and required finishing by hand. Drop the replace once the fix lands upstream.

Resource coverage today is intentionally narrow - only what entigo-infralib's Oracle
bootstrap (via `entigo-infralib-agent`) and its `oracle/vpc`, `oracle/dns`, `oracle/oke`,
and `oracle/oke-node-pool` terraform modules create:

* Networking: VCN, Subnet, Route Table (including a VCN's default route table),
  Internet Gateway, NAT Gateway, Service Gateway, Network Security Group,
  Load Balancer (created out-of-band by in-cluster controllers - the OCI CCM for
  Service type=LoadBalancer, the native ingress controller - so deleting the OKE
  cluster orphans them)
* DNS: Zone
* Object Storage: Bucket (including all object versions, not just current ones)
* Logging: Log Group, Log
* Compute: Container Instance
* Kubernetes Engine (OKE): Cluster, Node Pool
* DevOps: Project, Deploy Pipeline, Build Pipeline
* Notifications: ONS Topic
* IAM: Dynamic Group, Policy, Customer Secret Key, User, Group
* Certificates: Certificate (created out-of-band by the native ingress controller,
  one per TLS ingress, and never cleaned up by it), Certificate Authority
* Vault: Vault, Key

### Nothing in the crypto stack deletes immediately

OCI will only ever *schedule* deletion for a certificate, CA, vault or key, and it
enforces a minimum wait: 24 hours for a certificate ("less than minimum 1440 even after
allowable clock skew 5") and **7 days** for a CA, a vault or a key - the CA is the odd one
out, since it reads like a certificate but is held to a vault's patience ("minimum
10080"). `oci-nuke` asks for the earliest date each service accepts, its minimum plus the
five minutes of clock skew the certificates service says it allows for, rather than the
long default that comes of naming no date at all. A freshly nuked compartment therefore
still lists these in `PENDING_DELETION` until their date comes, and nothing blocks
re-provisioning in the meantime.

The date is read back rather than assumed. A schedule request returns 200 without
promising it kept the date it was given: a certificate asked for 24h05m came back holding
a date seven and a half days out, and one scheduled with no date at all comes back ten
days out, not the thirty the API documents. Each run reads the resource after scheduling
it, withdraws and re-issues a date that is further out than the one it asked for, and
fails the resource with the date OCI actually kept if it will not take ours.

That also means a schedule someone else made is not final. A certificate or CA deleted
without an explicit date - by the console, by a terraform destroy, or by an `oci-nuke`
build from before this was set - is picked up by a later run and pulled in to the earliest
allowed date. (Vaults and keys are not: a vault already pending deletion no longer serves
the management endpoint its keys are enumerated through.)

The run itself does not wait for any of these dates. A resource whose schedule is correct
stops being listed, which is how libnuke marks an item finished, so the nuke ends in the
time the API calls take - not the day or the week until the deletion is due.

It is tempting to exclude vaults, keys and CAs for that reason - an HSM key version is
billed for the whole seven days it spends waiting, so deleting it appears to buy nothing on
a same-day rebuild. **Don't, unless the rebuild can adopt what you kept.** A nuke that also
removes the terraform state leaves the survivors with nothing referencing them: the rebuild
creates a fresh vault, keys and CA under a new random suffix and the old set is orphaned.
Seven days of pending deletion costs about $0.12 per key version; an orphan costs about
$0.53 a month indefinitely, and you get another one every rebuild.

If you do want to keep them, an exclude in the config always beats an `--include` on the
command line, so it has to be commented out rather than overridden with a flag.

Note that the vault is not purely a terraform concern: `entigo-infralib-agent` creates its
own `<prefix>-infralib` vault and key during bootstrap, before any module runs.

As entigo-infralib's Oracle module set grows further, this list needs to keep growing with
it. Contributions and issues welcome.

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

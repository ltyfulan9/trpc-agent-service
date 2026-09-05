# Kubernetes rollout contract

These manifests are a hardened baseline, not a cluster-independent installer.
Apply them to a dedicated namespace with a CNI that enforces Kubernetes
`NetworkPolicy`.

Before enabling `network-policies.yaml`, label the namespaces that own the
approved ingress paths:

```bash
kubectl label namespace ingress-system agent-platform-access=public-ingress
kubectl label namespace observability agent-platform-access=observability
kubectl label namespace platform-operators agent-platform-access=admin-ingress
kubectl label namespace egress-system agent-platform-access=egress-gateway
kubectl label namespace data-plane agent-platform-access=data-plane
```

The Gateway service is intentionally `ClusterIP`. Public TLS, WAF/body limits
and the `/webhook` route belong to a separately managed Ingress or Gateway API
resource in `ingress-system`. Admin must use a different private ingress path
from `platform-operators`.

## Consumer-to-Worker transport

The Consumer defaults to `WORKER_TRANSPORT_MODE=production`. In that mode the
startup gate accepts only an `https://` Worker endpoint and returns a stable
configuration error before the Consumer can claim Inbox work. The Worker in
this source snapshot still serves plain HTTP; it does not implement
`ServeTLS` or application-level mTLS. A production deployment therefore needs
an HTTPS terminator with certificate verification, or an operator-managed
service mesh that provides the confidential hop.

The checked-in pipeline manifest keeps the Worker app hop at
`http://agent-worker:9090`, sets `WORKER_TRANSPORT_MODE=mesh`, and leaves
`WORKER_MESH_MTLS_ASSERTED=false`. This is intentionally fail-closed and is a
source template, not a deployable release. CI must render a separate release
bundle that either uses an `https://` endpoint in `production` mode or changes
the mesh assertion to `true` after strict peer authentication, identity
authorization, and NetworkPolicy enforcement have been verified. A mesh
release must also add a bounded `agent.trpc.io/mesh-mtls-evidence` annotation
that points to the reviewed change record. The assertion is a deployment
precondition; it does not add TLS to the Go Worker or prove that a mesh is
installed. `WORKER_TRANSPORT_MODE=development` is reserved for the isolated
Compose/local stack and must not be used in Kubernetes production.

HMAC service authentication remains enabled in every mode, but HMAC protects
integrity and replay resistance only. It is not a substitute for encryption:
plaintext HTTP can expose tenant identifiers and user prompts to a compromised
or misconfigured cluster network.

The checked-in policy assumes PostgreSQL and Redis pods are in the application
namespace and labelled `app=postgres` and `app=redis`. If managed private
services are used, replace those pod-selector egress rules with reviewed exact
private host routes (`/32` or `/128`) before applying default-deny; list every
approved endpoint separately. The release verifier rejects empty peers,
destination-free egress rules, namespace-only peers, broad private ranges,
link-local/metadata, loopback, public CIDRs, and `ipBlock` exceptions. Worker
and Delivery have no direct public CIDR
egress: they reach an `agent-egress-gateway` only in a namespace labelled
`agent-platform-access=egress-gateway`. That separately operated gateway owns
provider FQDN/SNI allowlists, its TLS policy, and external egress controls.
The rollout verifier rejects any public `ipBlock` (including split-world CIDRs)
and rejects a policy that omits the controlled egress path for either Worker or
Delivery.

When Linkerd native sidecars/CNI are used, inbound application traffic is
intercepted on proxy port 4143 before reaching the application port. The
reviewed `network-policies.yaml` therefore permits 4143 only on the same
identity/namespace boundaries as each workload's real application port and
permits the exact Linkerd control-plane egress path. Removing 4143 makes a
default-deny policy block otherwise valid meshed traffic; opening it broadly
would bypass the intended peer boundary. `network_policies_test.go` fixes this
contract.

The rollout script deliberately has no permissive fallback. It accepts only a
rendered release bundle, not the mutable source templates. The bundle must
contain these eight files with the exact workloads represented in aggregate:
`migration.yaml`, `profiles.yaml`, `availability-policies.yaml`, `worker.yaml`,
`summary.yaml`, `pipeline.yaml`, `admin.yaml`, and `gateway.yaml`. Every application image, including the
migration Job, must be an OCI `@sha256:` digest produced by the release build.
The bundle is verified before the script mutates the cluster; `mesh` transport
also requires an attested `true` assertion and evidence annotation.
The rendered migration Job must bind the reviewed rollout classification in
`agent.trpc.io/schema-class`. Its value must exactly match
`RELEASE_SCHEMA_CLASS`; a breaking bundle must additionally bind the approved
change record in `agent.trpc.io/breaking-change-id`. This catches artifact and
operator-input drift but does not decide compatibility: migration review still
owns that classification.
The verifier also treats the bundle as an allowlist, not an open YAML stream:
only the six application Deployments, migration Job, runtime profile ConfigMap,
three internal Services, three HPAs, and six PodDisruptionBudgets are accepted, all at the expected API
versions and without an embedded namespace. Missing, duplicate, or extra
objects fail verification before any cluster mutation. Ingress/Gateway API,
RBAC, Secrets, and cluster observability remain separately operated overlays;
they must not be smuggled into this application release bundle.

Build `cmd/releaseverify` in the pinned release environment, or let the script
use its local Go toolchain with `GOTOOLCHAIN=local`. Supply the reviewed policy
and the rendered bundle explicitly:

```bash
PLATFORM_NAMESPACE=agent-platform \
RELEASE_MANIFEST_DIR=/secure/release/2026-08-27.1 \
NETWORK_POLICY_FILE=deploy/kubernetes/network-policies.yaml \
RELEASE_SCHEMA_CLASS=compatible \
./scripts/k8s_apply.sh
```

For managed databases, point `NETWORK_POLICY_FILE` at an environment overlay
with exact DB/Redis CIDRs. The script also refuses rollout unless all three
access namespace labels, the egress-gateway namespace label, and all required
Secrets (including the OTLP CA bundle) exist.

`PLATFORM_NAMESPACE` is intentionally mandatory; the script never falls back
to the current context or Kubernetes `default` namespace. It also refuses to
delete a running or failed `agent-migrate` Job. Resolve that Job explicitly;
only a completed migration Job is replaced for the next checksum-verified
release.

`audit-identity` is a shared high-entropy HMAC key required by Gateway and
Worker so the same external user gets a stable tenant-scoped pseudonym without
storing the raw provider identity. Provision and rotate it through the same
external secret manager as the service credentials.

`metrics-auth` is also required by every HTTP workload. If the cluster uses
Prometheus Operator, apply `monitoring-example.yaml` after its `PodMonitor` CRD
is installed and configure the Prometheus resource to select that object and
its namespace. The example reads the bearer token from `metrics-auth`; it is
not applied by `k8s_apply.sh` because monitoring CRDs and selector policies are
cluster-owned. Without an equivalent authenticated scrape configuration,
metrics availability has not been established.

Tenant labels on Prometheus metrics are bounded by default: every tenant is
aggregated as `__other__` and a missing ID as `__unknown__`. To retain exact
labels for a small, reviewed set, inject `METRICS_TENANT_ALLOWLIST` as a
comma-separated list (maximum 100 IDs) into every HTTP workload. Keep the
allowlist small. Exact `agent_name` and `model` labels additionally require
`METRICS_AGENT_ALLOWLIST` and `METRICS_MODEL_ALLOWLIST` entries in
`tenant/name` form (maximum 200 pairs per dimension). A tenant allowlist entry
alone never enables runtime names, so version rotation cannot accumulate
unbounded Prometheus series. Use logs or a cost warehouse for broad reporting.

Every HTTP workload also requires the `otel-collector-tls` Secret in the
application namespace. Its `ca.crt` key must contain the CA that signs the
collector certificate; the collector certificate must include
`otel-collector.observability.svc` in its SANs. Workloads keep
`OTEL_EXPORTER_OTLP_INSECURE=false` and mount this CA read-only at
`/var/run/secrets/otel/ca.crt`. Deploy the collector with a TLS-enabled OTLP
gRPC receiver before rollout. Do not change the flag to `true` as a workaround:
that would silently send traces without transport encryption. If a service mesh
terminates TLS, use its documented trust bundle and endpoint instead, while
preserving an authenticated, encrypted contract.

Gateway and Worker HPAs use `ContainerResource` metrics for their named
application containers. Do not convert them back to Pod-level `Resource`
metrics without giving every injected sidecar matching requests: otherwise a
service-mesh sidecar can make the aggregate utilization unknown and silently
disable scaling. A live rollout must show `ScalingActive=True` and
`ValidMetricFound`.

The Consumer manifest explicitly enables the PostgreSQL fair queue and fixes
`CONCURRENCY=4`. This is the locally measured safe starting profile for two
Consumer Pods (eight claim workers total). Raising either replicas or per-Pod
concurrency requires rerunning the fair-claim contention and durability
scenario against the target PostgreSQL specification.

`ADMIN_API_TOKEN` is the emergency/bootstrap platform administrator. Additional
operators come from the optional `principals-json` secret key and should be
issued per-person, tenant-scoped credentials. In production, generate both
secrets through an external secret operator and rotate them; the example Secret
objects are deliberately non-deployable placeholders.

Tenant Session/Memory connections come from `tenant-storage-credentials`, not
from Tenant JSON. Only Worker receives that Secret because it alone constructs
the Session/Memory data plane; Gateway, Admin, Consumer and Delivery validate
only public profile metadata. `STORAGE_BACKEND_PROFILES` maps public profile
IDs to secret environment variable names. Use separate least-privilege database
roles and Redis ACL users for these profiles; do not point them at the
control-plane owner credential.

Knowledge/Artifact runtime endpoints and non-secret options come from the
rendered `runtime-data-plane-profiles` ConfigMap (`profiles.json`). The same
public catalog is loaded by all six deployments so invalid tenant bindings are
rejected consistently, but only the Worker receives
`runtime-data-plane-credentials` and resolves Qdrant, embedding and S3 secret
environment variables. `releaseverify` rejects unknown profile fields, inline
secret values, invalid insecure endpoints, missing bindings, duplicate refs,
or any credential reference in a non-Worker/sidecar container. Qdrant and
object-storage namespaces must carry `agent-platform-access=data-plane`; the
checked-in NetworkPolicy exposes only the selected application and Linkerd
proxy ports. Replace the example selectors/ports with the exact managed
private routes for the target environment.

Operator-owned MCP declarations use `mcp-profiles.json` in the same ConfigMap.
Only Admin (admission) and Worker (execution) receive that public catalog; only
Worker receives the referenced `mcp-profile-credentials`. Profiles must expose
an exact remote-tool allowlist and use HTTPS in production; `stdio` is rejected
because a tenant-selected local process would cross the container boundary.
`channel-credentials` is mounted only into Gateway/Delivery, while
`model-provider-credentials` is mounted only into Worker/Summary Worker. All
entries are optional at Pod startup and fail closed only when a tenant actually
references a missing SecretRef. Replace the example Secrets with workload
identity/external-secret projections and review each provider egress route.

Migration 018 changes execution rows to append-only fenced attempts and is not
write-compatible with an active application data plane. Prefer expand/contract
migrations; when a genuinely breaking migration is unavoidable, first close
public intake at the managed edge, drain queues and active leases, and stop all
Gateway, Consumer, Worker, Delivery, and Admin replicas. This creates a full
database write-silence window: Gateway writes Inbox, Consumer advances Inbox,
Worker writes execution/Session/Memory state, Delivery advances Outbox, and
Admin mutates control-plane records. Run the migration Job only after all five
workloads have no remaining Pods, then restore the new downstream workloads
before reopening Gateway and edge intake.
For this class of release, call the script with
`RELEASE_SCHEMA_CLASS=breaking`, `BREAKING_MIGRATION_APPROVED=true`, a bounded
`BREAKING_MIGRATION_CHANGE_ID`, and a non-empty
`BREAKING_MIGRATION_DRAIN_EVIDENCE` artifact. The script refuses to run until
all existing application Deployments are scaled to zero with no Pods; it also
rejects orphan Pods whose Deployment is already absent. Bootstrap migrations
likewise require that none of these Deployments or Pods exists.
The evidence file is an auditable operator record, not fabricated proof that a
queue was drained; retain the real queue/lease checks with the change record.

Rollout order is migration Job, PDBs, Worker/Admin, Consumer/Delivery, then
Gateway. `k8s_apply.sh` waits for Worker/Admin availability before submitting
Consumer/Delivery, then waits for the pipeline before applying Gateway; a
successful `kubectl apply` alone is not rollout evidence. Confirm default-deny
does not break DNS, database, Redis or OTLP in a staging namespace before
production cutover.

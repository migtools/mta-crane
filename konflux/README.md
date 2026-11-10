# MTA-Ops Konflux Integration Test

This directory contains the Konflux-facing integration-test implementation for
MTA-Ops Tier 0 testing. It is intentionally separate from the Go source code
and from local/manual test experiments.

## Directory Layout

```text
konflux/
  pipelines/
    mta-ops-konflux-tier0.yaml
  tasks/
    mta-ops-extract-image-references.yaml
    mta-ops-resolve-art-nvr.yaml
    mta-ops-provision-one-fresh-cluster.yaml
    mta-ops-ocpctl-download-kubeconfig.yaml
    mta-ops-delete-fresh-clusters.yaml
    mta-ops-merge-kubeconfigs.yaml
    mta-ops-setup-nonadmin-contexts.yaml
    mta-ops-verify-contexts.yaml
    mta-ops-prepare-tools.yaml
    mta-ops-extract-image.yaml
    mta-ops-run-tier0.yaml
  pipelineruns/
    mta-ops-konflux-tier0.yaml
  integrationtests/
    mta-ops-its.yaml
```

## Konflux Flow

```text
Konflux Snapshot
  -> extract MTA-Ops and rsync image references
  -> optional ART NVR lookup
  -> provision source cluster (OCP 4.21)
  -> provision target cluster (OCP 4.22)
  -> download both kubeconfigs
  -> merge src/tgt contexts
  -> create src-dev/tgt-dev contexts
  -> verify all contexts
  -> prepare tests and k8sdeploy
  -> extract MTA-Ops binary
  -> run Tier 0
  -> delete both clusters after the debug delay
```

The image-reference extraction runs before provisioning clusters. It reads the
Konflux `SNAPSHOT` parameter and locates:

- A component whose name ends in `-mta-ops`.
- A component whose name ends in `-mta-rsync-transfer`.

The Snapshot image references are converted to the corresponding stage
repositories while preserving their digests:

- `mta-ops` -> `registry.stage.redhat.io/mta/mta-ops-rhel9@sha256:<digest>`
- `mta-rsync-transfer` -> `registry.stage.redhat.io/mta/mta-rsync-transfer-rhel9@sha256:<digest>`

The stage images are verified before cluster resources are allocated. Verification
currently retries for up to one minute while debugging the ITS; the retry window
is configurable through the Task parameters. The test clusters already have
access to the stage registry, so the pipeline does not modify their pull secrets.

## IntegrationTestScenario

The manifest is:

```text
konflux/integrationtests/mta-ops-its.yaml
```

It is configured for the Application and component context that own MTA-Ops:

```yaml
spec:
  application: mta-8-3
  contexts:
    - name: component_mta-8-3-mta-ops
```

The ITS resolves a PipelineRun, not a Pipeline, because the PipelineRun must
bind the shared PVC workspace:

```yaml
resourceKind: pipelinerun
pathInRepo: konflux/pipelineruns/mta-ops-konflux-tier0.yaml
revision: konflux-its
```

The Git resolver paths are exact. If files move, update all `pathInRepo`
values in the ITS, PipelineRun wrapper, and Pipeline Task resolvers together.

## Required Configuration

The following values are intentionally not stored in this repository:

- OCPCTL API key.
- Registry credentials.
- Cluster URLs or passwords.

Configure them in the Konflux ITS/PipelineRun environment:

| Value | Purpose |
| --- | --- |
| `ocpctlUrl` | OCPCTL API base URL; defaults to the shared OCPCTL service endpoint |
| `ocpctlSecretName` | Kubernetes Secret containing the API key; default name is `ocpctl-ci-key` |
| `ocpctlSecretKey` | Secret key containing the API key; default is `api-key` |
| `artBuildHistoryUrl` | Optional ART API URL for NVR logging |
| `artGroup` | Optional ART group, for example `mta-8.3` |

The OCPCTL Secret must exist in the same tenant namespace as the PipelineRun.
It must contain an `api-key` entry. Do not commit the key or a cluster URL to
Git.

## Adding Or Updating The ITS

1. Ensure the MTA-Ops component belongs to the selected Application.
2. Use the exact context `component_mta-8-3-mta-ops` to trigger only for MTA-Ops component builds.
3. Use `resourceKind: pipelinerun` because the wrapper binds `shared-workspace`.
4. Set the Git resolver revision to `konflux-its`.
5. Set the path to `konflux/pipelineruns/mta-ops-konflux-tier0.yaml`.
6. Create the OCPCTL Secret in the tenant configuration; override `ocpctlUrl`
   only when using a different OCPCTL service.
7. Optionally provide `artBuildHistoryUrl` and `artGroup` to log NVRs.
8. Trigger a new MTA-Ops component build or retest after changing the ITS.

Creating or editing the ITS does not retroactively run an existing Snapshot.
A new matching Snapshot is required.

## OCPCTL Cluster Access

The pipeline provisions two fresh clusters through OCPCTL using the
`aws-sno-ga` profile, source version `4.21`, and target version `4.22`.
Jenkins uses these ownership values for the flow:

```text
team: CI-Automation
cost center: 773
region: us-east-1
```

 The owner is resolved from the authenticated OCPCTL user. Clusters are
deleted in `finally` after the configured debug delay, which is 60 seconds by
default.

The Task pods must be able to resolve and reach the OCPCTL endpoint. If the
tenant cluster cannot resolve the internal OCPCTL hostname, provisioning will
fail before authentication.

## Workspace And Cleanup

The PipelineRun wrapper creates a persistent `shared-workspace` PVC because
Tasks run in separate pods. The workspace contains kubeconfigs, source files,
tools, and test reports.

Clusters are deleted in `finally`, including when provisioning or testing
fails. The delete Task uses the cluster IDs returned by the provision Task.
Completed PVC cleanup is still controlled by the platform retention policy and
should be reviewed separately.

## Test Execution

The final Task currently runs:

```bash
ginkgo run -v -r --procs=4 \
  e2e-tests/tests/tier0 -- \
  --source-context=src \
  --target-context=tgt \
  --source-nonadmin-context=src-dev \
  --target-nonadmin-context=tgt-dev \
  --rsync-image=<resolved-rsync-image>
```

The MTA-Ops binary is extracted from the Snapshot-resolved Stage image. The
rsync image is passed to the test suite as `--rsync-image`.

The Task emits the standardized `TEST_OUTPUT` result. JUnit and logs remain
on the workspace for now; durable publishing is future work.

## Troubleshooting

### No PipelineRun is created

Check that:

- The ITS Application is `mta-8-3`.
- The context is `component_mta-8-3-mta-ops`.
- A new MTA-Ops component Snapshot was created after the ITS update.
- The Git resolver revision and PipelineRun path are correct.

### Snapshot image extraction fails

Check that the Snapshot contains both:

```text
*-mta-ops
*-mta-rsync-transfer
```

The extraction Task requires stage registry credentials through the configured
`registry-pull-secrets` Secret and the Konflux service account credentials.

### Lease fails before authentication

Check OCPCTL DNS and network reachability from tenant Task pods. Also check
the `ocpctl-ci-key` Secret name and `api-key` entry.

### Non-admin setup fails

Inspect the source and target kubeconfig access first. The Task waits for admin
authentication before changing OAuth configuration. A target kubeconfig that
returns HTTP 401 indicates an OCPCTL lease/kubeconfig readiness issue, not a
test RBAC failure.

## Future Work

- Replace the temporary fixed test assumptions with full Snapshot/version mapping.
- Add durable JUnit and log publishing.
- Add automatic PVC retention and cleanup policy.
- Expand from Tier 0 to Tier 1 and upgrade coverage.
- Add Slack or another team notification only for actionable failures.

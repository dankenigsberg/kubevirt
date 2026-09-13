---
title: "refactor: Replace Root feature gate with root-launcher Plugin"
type: refactor
status: active
date: 2026-09-12
deepened: 2026-09-12
---

# Replace Root Feature Gate with Root-Launcher Plugin

## Overview

Remove the cluster-wide `Root` feature gate and **all rootfulness-related
code branches** from the codebase — including but not limited to `IsNonRoot()`
and every `RuntimeUser == 0` comparison. The base code becomes unconditionally
non-root with zero root/non-root branching. All root-specific behavior —
security context, capabilities, libvirt URI, file paths, device ownership rules
— is carried by a built-in `root-launcher` Plugin CR, delivered through a new
`LauncherPodHooks` extension to the Plugin CRD.  `LauncherPodHooks` carries a
partial `PodTemplateSpec` that is strategic-merge-patched into the rendered
virt-launcher pod, giving any plugin author the ability to override any pod
spec field — analogous to how `DomainHooks` let plugins override any domain XML
element.

## Problem Frame

Today root-specific logic is scattered across 20+ callsites behind
`IsNonRoot()`, `RootEnabled()`, and `--run-as-nonroot` branches in
virt-controller, virt-handler, virt-launcher, and supporting packages. The
`Root` feature gate is a cluster-wide all-or-nothing switch. This design has
two problems: (1) no per-VMI granularity, and (2) the root behavior is
entangled with the core code rather than being a separable concern. A
plugin-based approach makes root a modular, opt-in capability that ships its
own logic.

## Requirements Trace

- R1. A VM owner can request root virt-launcher for a specific VMI via the
      `kubevirt.io/nonroot: "false"` annotation.
- R2. A root virt-launcher pod runs only if the namespace permits it,
      e.g. if it has the Pod Security Admission (PSA) label
      `pod-security.kubernetes.io/enforce: privileged` set.
      KubeVirt does not proactively check this label, as that would be raceful.
      If the namespace lacks the label, the pod is rejected by the API server and the
      existing reactive handler in `lifecycle.go:175-178` surfaces a helpful error on
      the VMI.
- R3. VMIs without the annotation default to non-root (UID 107).
- R4. The `Root` feature gate changes meaning: it controls whether the
      `root-launcher` Plugin CR is deployed by virt-operator. When enabled,
      root-via-annotation is available. When disabled (default), no root
      path exists. It remains Alpha.
- R5. All rootfulness-related code branches are removed — `IsNonRoot()`,
      `--run-as-nonroot` and its downstream `nonRoot bool` branches, and
      `RuntimeUser == 0` comparisons. The base code has zero root/non-root
      branching. `RootEnabled()` is retained solely for the operator to
      decide whether to deploy the root-launcher Plugin CR.
- R6. `DeprecatedNonRootVMIAnnotation` is revived as the canonical opt-in
      mechanism. Setting `kubevirt.io/nonroot: "false"` triggers the
      root-launcher Plugin. The "Deprecated" prefix is dropped.
- R7. Live migration preserves the VMI's annotation and RuntimeUser.
- R8. Seamless upgrade for clusters using the Root feature gate: VM that runs as root, would be annotated kubevirt.io/nonroot=false.
- R9. The root-launcher capability is delivered as a Plugin CR using a new
      `LauncherPodHooks` extension to the Plugin CRD. `LauncherPodHooks`
      carries a partial `corev1.PodTemplateSpec` that is strategic-merge-
      patched into the rendered pod. Any plugin author can use it to
      override any virt-launcher pod spec field.

## Scope Boundaries

- Only UID 0 is intended to be supported.
- `LauncherPodHooks` carries a partial `corev1.PodTemplateSpec` — any pod
  spec field can be overridden. No validation beyond standard Kubernetes
  PodTemplateSpec schema; we trust the plugin author.
- No changes to SCC objects. Namespace PSA labels are the enforcement
  boundary.

## Context & Research

### Complete Rootfulness Branch Map

Every root/non-root branch in the codebase, grouped by mechanism:

**A. `IsNonRoot()` callsites (12 sites in virt-controller / virt-handler)**

| # | File | What it does for root | Elimination |
|---|---|---|---|
| 1 | `template.go:367` | `computePodSecurityContext()` — RunAsUser=0 | LauncherPodHooks patch |
| 2 | `template.go:388` | `renderLaunchManifest()` — userId=0, omit `--run-as-nonroot` | LauncherPodHooks patch |
| 3 | `template.go:885` | Sidecar container — skip `WithNonRoot`, skip `DropALL` | LauncherPodHooks patch |
| 4 | `template.go:908` | Init container — skip `WithNonRoot` | LauncherPodHooks patch |
| 5 | `template.go:926` | Compute container — skip `WithNonRoot`, skip `DropALL` | LauncherPodHooks patch |
| 6 | `rendercontainer.go:296` | Add `CAP_SYS_NICE` | LauncherPodHooks patch |
| 7 | `rendervolumes.go:873` | swtpm-localca path | LauncherPodHooks patch |
| 8 | `controller.go:350` | Skip `nonRootSetup()` (device chown) | Always chown to 107 |
| 9 | `launcher-clients.go:226` | Socket ownership | Always chown to 107 |
| 10 | `live-migration-source.go:1358` | `qemu:///system` URI | LauncherPodHooks patch |
| 11 | `netconf.go:99` | Network device ownerID | Always chown to 107 |
| 12 | `cbt.go:324` | CBT directory path | LauncherPodHooks patch |

**B. `RootEnabled()` feature gate checks (2 sites)**

| # | File | What it does | Elimination |
|---|---|---|---|
| 13 | `vmi-mutator.go:86` | `if !clusterConfig.RootEnabled()` → `markAsNonroot()` | Unconditional `markAsNonroot()` (Unit 5) |
| 14 | `migration.go:1943` | `if !c.clusterConfig.RootEnabled()` → `setupVMIRuntimeUser()` | Unconditional RuntimeUser=107 (Unit 5) |

**C. `--run-as-nonroot` flag and `nonRoot bool` branches (11 sites in virt-launcher)**

| # | File | What it does | Elimination |
|---|---|---|---|
| 15 | `virt-launcher.go:357` | Flag definition: `pflag.Bool("run-as-nonroot")` | Remove flag (Unit 3) |
| 16 | `virt-launcher.go:394` | Console log init branch | Env var: `VIRT_LAUNCHER_LOG_DIR` |
| 17 | `virt-launcher.go:422` | `NewLibvirtWrapper(*runWithNonRoot)` | Env var: `VIRT_LAUNCHER_UID` |
| 18 | `virt-launcher.go:432` | `StartVirtlog(nonRoot)` | Env var: `VIRT_LAUNCHER_LOG_DIR` |
| 19 | `virt-launcher.go:434` | `createLibvirtConnection(nonRoot)` — URI + user | Env var: `VIRT_LAUNCHER_LIBVIRT_URI` |
| 20 | `virt-launcher.go:508` | `startDomainEventMonitoring(nonRoot)` | Env var: `VIRT_LAUNCHER_LOG_DIR` |
| 21 | `virt-launcher.go:533` | PID dir selection | Env var: `VIRT_LAUNCHER_PID_DIR` |
| 22 | `libvirt_helper.go:103` | `NewLibvirtWrapper(nonRoot)` — UID for virtqemud | Env var: `VIRT_LAUNCHER_UID` |
| 23 | `libvirt_helper.go:267` | `if l.user != 0` — AmbientCaps for virtqemud | Env var: `VIRT_LAUNCHER_UID` |
| 24 | `libvirt_helper.go:319` | `GetQemuLogPath(nonRoot)` — log path | Env var: `VIRT_LAUNCHER_LOG_DIR` |
| 25 | `libvirt_helper.go:536` | `if !l.root()` — qemu.conf path | Env var: `VIRT_LAUNCHER_UID` |

**D. `RuntimeUser` comparisons (2 sites in migration controller)**

| # | File | What it does | Elimination |
|---|---|---|---|
| 26 | `migration.go:1946` | `if vmi.Status.RuntimeUser != util.NonRootUID` | Unconditional RuntimeUser=107 (Unit 5) |
| 27 | `migration.go:1959` | `if vmi.Status.RuntimeUser != util.RootUser` | Unconditional RuntimeUser=107 (Unit 5) |

**E. `DeprecatedNonRootVMIAnnotation` manipulation (2 sites in migration controller)**

| # | File | What it does | Elimination |
|---|---|---|---|
| 28 | `migration.go:1952` | Adds `DeprecatedNonRootVMIAnnotation: "true"` | Remove entirely (Unit 5) |
| 29 | `migration.go:1963` | Removes `DeprecatedNonRootVMIAnnotation` | Remove entirely (Unit 5) |

**F. `template.go` root-specific arg injection (1 site)**

| # | File | What it does | Elimination |
|---|---|---|---|
| 30 | `template.go:453` | `if nonRoot { args = append("--run-as-nonroot") }` | Remove with flag (Unit 3) |

**Elimination strategy — four tracks:**

- **LauncherPodHooks track (callsites 1-7, 10, 12):** The plugin carries
  a partial `PodTemplateSpec` that is strategic-merge-patched into the
  rendered pod. Security context, capabilities, env vars — all injected
  through the patch. The base code reads from env vars / pod spec — no
  branches.

- **Env-var track (callsites 15-25, 30):** The `--run-as-nonroot` flag
  and all downstream `nonRoot bool` parameters are replaced by env vars
  with non-root defaults. The root-launcher Plugin injects the root
  values via LauncherPodHooks. virt-launcher reads env vars without
  branching.

- **Always-chown track (callsites 8, 9, 11):** These callsites always
  chown to UID 107. Root (UID 0) bypasses file permissions, so files
  owned by 107 are fully accessible to root. Unconditional — no
  branching, no RuntimeUser lookup.

- **Unconditional-107 track (callsites 13, 14, 26-29):** The feature gate
  checks and RuntimeUser comparisons in the webhook and migration controller
  are replaced by unconditional `RuntimeUser=107`. The plugin's own
  mutating webhook overrides to 0 when needed — core code has zero
  root awareness.

### Plugin CRD Architecture

- Plugin CRD: `staging/src/kubevirt.io/api/plugin/v1alpha1/types.go`
- CEL conditions evaluated against VMI: `plugins/cel/evaluator.go`
- Plugin evaluation pipeline: `plugins/pipeline.go`
- Plugin informer: `virtinformers.go:887`
- Plugins consumed by virt-handler: `controller.go:187-200`
- virt-controller currently does NOT read Plugin CRs

### Key File Paths

- Pod rendering: `pkg/virt-controller/services/template.go`
- Container rendering: `pkg/virt-controller/services/rendercontainer.go`
- Volume rendering: `pkg/virt-controller/services/rendervolumes.go`
- virt-handler device setup: `pkg/virt-handler/non-root.go`
- virt-handler controller: `pkg/virt-handler/controller.go`
- Socket ownership: `pkg/virt-handler/launcher-clients/launcher-clients.go`
- Migration source: `pkg/virt-launcher/virtwrap/live-migration-source.go`
- Network setup: `pkg/network/setup/netconf.go`
- CBT: `pkg/storage/cbt/cbt.go`
- virt-launcher main: `cmd/virt-launcher/virt-launcher.go`
- VMI trait: `pkg/vmitrait/vmitrait.go`
- Feature gates: `pkg/virt-config/featuregate/active.go`
- VMI mutator: `pkg/virt-api/webhooks/mutating-webhook/mutators/vmi-mutator.go`
- Migration controller: `pkg/virt-controller/watch/migration/migration.go`

### Why LauncherPodHooks Instead of a Webhook

A MutatingAdmissionPolicy on the pod could change RunAsUser but not the
env vars, capabilities, and other coordinated fields that virt-launcher
needs at runtime. The TemplateService must render these together from a
single source. LauncherPodHooks let the Plugin strategic-merge-patch the
full PodTemplateSpec coherently — and unlike a structured override API,
any plugin author can override any pod spec field without KubeVirt
having to anticipate every use case.

## Key Technical Decisions

- **Extend Plugin CRD with `LauncherPodHooks`**: A partial
  `corev1.PodTemplateSpec` that is strategic-merge-patched into the rendered
  virt-launcher pod. Applied by the TemplateService after base rendering.
  Any pod spec field can be overridden — analogous to how `DomainHooks`
  let plugins override any domain XML element. No custom validation; we
  trust the plugin author.

- **Config-driven virt-launcher**: Replace the `--run-as-nonroot` flag and
  all hardcoded path selection with env vars. The root-launcher Plugin
  injects env vars that override the defaults. virt-launcher reads them
  without branching:
  - `VIRT_LAUNCHER_LIBVIRT_URI` (default: `qemu+unix:///session`)
  - `VIRT_LAUNCHER_LOG_DIR` (default: `/var/run/kubevirt-private/libvirt/qemu/log/`)
  - `VIRT_LAUNCHER_PID_DIR` (default: `/run/libvirt/qemu/run`)
  - `VIRT_LAUNCHER_SWTPM_DIR` (default: `/var/run/kubevirt-private/var/lib/swtpm-localca`)
  - `VIRT_LAUNCHER_CBT_DIR` (default: `/var/run/kubevirt-private/libvirt/qemu/cbt`)

- **Unconditional virt-handler**: `nonRootSetup()` becomes
  `setupDeviceOwnership()` and always chowns to UID 107. Root bypasses
  file permissions so this is safe for both root and non-root VMIs.
  No RuntimeUser lookup, no branching.

- **Annotation as VMI-side opt-in**: `kubevirt.io/nonroot: "false"` on the
  VMI. Revives the existing `DeprecatedNonRootVMIAnnotation` with a value
  semantic: absence or `"true"` means non-root (default), `"false"` means
  root. Immutable after creation.

- **Root feature gate repurposed**: Stays in `active.go` as Alpha. Its
  meaning changes from "all VMs run as root" to "deploy the root-launcher
  Plugin CR." When enabled, virt-operator installs the Plugin; when
  disabled (default), no root path exists.

## Open Questions

### Resolved During Planning

- **Q: Should KubeVirt proactively validate namespace PSA labels?**
  A: No. PSA enforces at pod creation. The reactive handler surfaces a
  helpful error. No extra machinery needed.

- **Q: Can root-specific behavior be fully eliminated from base code?**
  A: Yes. Pod-level differences are handled by LauncherPodHooks. Runtime-level
  differences (paths, libvirt URI) are handled by env vars. Device ownership
  always uses UID 107. RuntimeUser override and legacy upgrade path are
  handled by the plugin's own mutating webhook. Core code has zero root
  awareness.

- **Q: Should LauncherPodHooks use CEL, structured fields, or a patch?**
  A: Strategic merge patch on `corev1.PodTemplateSpec`. This is the most
  generic option — any pod spec field can be overridden, just like
  `DomainHooks` can modify any domain XML element. No custom validation
  needed; we trust the plugin author.

### Deferred to Implementation

- Exact env var names and defaults — the names above are directional.
- Strategic merge patch semantics for list fields (containers are patched
  by name; env vars are appended or replaced by name).

## High-Level Technical Design

> *Directional guidance, not implementation specification.*

```
                    Plugin CR "root-launcher"
                    ┌─────────────────────────────────────────┐
                    │ condition: 'has(vmi.annotations[        │
                    │   "kubevirt.io/nonroot"]) &&            │
                    │   vmi.annotations[...] == "false"'      │
                    │                                         │
                    │ launcherPodHooks:                       │
                    │   # partial PodTemplateSpec (SMP)       │
                    │   spec:                                 │
                    │     securityContext:                    │
                    │       runAsUser: 0                      │
                    │       runAsNonRoot: false               │
                    │     containers:                         │
                    │     - name: compute                     │
                    │       securityContext:                  │
                    │         capabilities:                   │
                    │           add: [SYS_NICE]               │
                    │         allowPrivilegeEscalation: true  │
                    │       env:                              │
                    │       - name: VIRT_LAUNCHER_LIBVIRT_URI │
                    │         value: qemu+unix:///system      │
                    │       - name: VIRT_LAUNCHER_LOG_DIR     │
                    │         value: /var/log/libvirt/qemu/   │
                    │       ... (PID_DIR, SWTPM_DIR, CBT_DIR) │
                    └───────────────┬─────────────────────────┘
                                    │
VMI CREATE ──► Core Mutating Webhook
               │  Always set RuntimeUser=107
               │
               ▼
           Plugin Mutating Webhook  │
               │                    │
               │  Check annotation ─┘
               │  Override RuntimeUser=0
               │
               ▼
           Template Service
               │
               │  1. Render base pod (always non-root defaults)
               │  2. Strategic-merge-patch LauncherPodHooks
               │     from matching Plugins
               │  No IsNonRoot() branches
               │
               ▼
           virt-handler
               │
               │  setupDeviceOwnership()
               │  Always chown to 107 — root bypasses permissions
               │  No IsNonRoot() branches
               │
               ▼
           virt-launcher
               │
               │  Read config from env vars:
               │    LIBVIRT_URI, LOG_DIR, PID_DIR, etc.
               │  No --run-as-nonroot flag
               │  No IsNonRoot() branches
```

## Implementation Units

### Phase 1: Plugin CRD and Infrastructure

- [ ] **Unit 1: Extend Plugin CRD with `LauncherPodHooks`**

  **Goal:** Add `LauncherPodHooks` to the Plugin CRD spec — a partial
  `corev1.PodTemplateSpec` that is strategic-merge-patched into the
  rendered virt-launcher pod.

  **Requirements:** R9

  **Dependencies:** None

  **Files:**
  - Modify: `staging/src/kubevirt.io/api/plugin/v1alpha1/types.go`

  **Approach:**
  - Add `LauncherPodHooks` field to `PluginSpec` typed as
    `corev1.PodTemplateSpec`. This is the partial patch that gets
    strategic-merge-patched into the rendered pod.
  - No custom validation — we trust the plugin author, just like
    `DomainHooks` trusts domain XML modifications.
  - Follow the `DomainHooks`/`NodeHooks` field naming pattern.
  - Run code generation (deepcopy, openapi).

  **Patterns to follow:**
  - `DomainHooks`/`NodeHooks` field structure in `types.go`

  **Test scenarios:**
  - Plugin with LauncherPodHooks carrying security context → accepted
  - Plugin with LauncherPodHooks carrying env vars → accepted
  - Plugin without LauncherPodHooks → accepted (backward compatible)

  **Verification:**
  - CRD schema accepts the new field
  - Existing Plugin tests still pass

### Phase 2: Make virt-launcher Config-Driven

- [ ] **Unit 2: Replace `--run-as-nonroot` with env-var-driven config**

  **Goal:** virt-launcher reads all root-sensitive config from env vars
  with non-root defaults, eliminating all branches on the `--run-as-nonroot`
  flag.

  **Requirements:** R5

  **Dependencies:** None (parallel with other phases)

  **Files:**
  - Modify: `cmd/virt-launcher/virt-launcher.go`
  - Modify: `pkg/virt-launcher/virtwrap/util/libvirt_helper.go`
  - Modify: `pkg/virt-launcher/notify-client/client.go`
  - Modify: `pkg/virt-launcher/virtwrap/live-migration-source.go`
  - Modify: `pkg/storage/cbt/cbt.go`
  - Modify: `pkg/virt-controller/services/rendervolumes.go`

  **Approach:**
  - Define env var names with non-root defaults:
    - `VIRT_LAUNCHER_LIBVIRT_URI` → default `qemu+unix:///session`
    - `VIRT_LAUNCHER_LOG_DIR` → default non-root path
    - `VIRT_LAUNCHER_PID_DIR` → default non-root path
    - `VIRT_LAUNCHER_SWTPM_DIR` → default non-root path
    - `VIRT_LAUNCHER_CBT_DIR` → default non-root path
    - `VIRT_LAUNCHER_UID` → default `107`
  - Remove the `--run-as-nonroot` flag definition and all downstream
    `nonRoot bool` / `*runWithNonRoot` branches:
    - `virt-launcher.go:394` — console log init
    - `virt-launcher.go:422` — `NewLibvirtWrapper(nonRoot)`
    - `virt-launcher.go:432` — `StartVirtlog(nonRoot)`
    - `virt-launcher.go:434` — `createLibvirtConnection(nonRoot)` (URI + user)
    - `virt-launcher.go:508` — `startDomainEventMonitoring(nonRoot)`
    - `virt-launcher.go:533` — PID dir selection
    - `libvirt_helper.go:103-112` — `NewLibvirtWrapper()` UID selection
    - `libvirt_helper.go:267` — AmbientCaps for virtqemud
    - `libvirt_helper.go:319-324` — `GetQemuLogPath()` log path
    - `libvirt_helper.go:536` — qemu.conf path
    - `client.go:307` — `GetQemuLogPath()` for panic log
  - Each branch reads from env vars with non-root defaults instead.
  - `pathForSwtpmLocalca()` in rendervolumes.go and CBT path in
    `cbt.go` read from env vars instead of branching on `IsNonRoot()`.

  **Test scenarios:**
  - virt-launcher with no env vars → uses non-root defaults
  - virt-launcher with root env vars → uses root paths/URI
  - Migration uses correct libvirt URI from env var

  **Verification:**
  - `grep -rE "run-as-nonroot|run_as_nonroot|runAsNonRoot|runWithNonRoot|nonRoot bool" cmd/ pkg/virt-launcher/` → zero hits
  - virt-launcher starts correctly with default env vars

### Phase 3: Make virt-handler Data-Driven

- [ ] **Unit 3: Replace `IsNonRoot()` in virt-handler with unconditional chown to 107**

  **Goal:** virt-handler always chowns devices to UID 107 instead of
  branching on `IsNonRoot()`. Root (UID 0) bypasses file permissions,
  so files owned by 107 are accessible to both root and non-root VMIs.

  **Requirements:** R5

  **Dependencies:** None (parallel with other phases)

  **Files:**
  - Modify: `pkg/virt-handler/non-root.go`
  - Modify: `pkg/virt-handler/controller.go`
  - Modify: `pkg/virt-handler/launcher-clients/launcher-clients.go`
  - Modify: `pkg/network/setup/netconf.go`

  **Approach:**
  - `nonRootSetup(vmi)` → `setupDeviceOwnership(vmi)` — always runs,
    always chowns to hardcoded UID 107. No RuntimeUser lookup.
  - `launcher-clients.go:226` — always set socket ownership to 107
    instead of branching.
  - `netconf.go:99` — always set network device ownerID to 107 instead
    of branching.
  - Remove the `IsNonRoot()` guard at `controller.go:350` — device
    ownership setup always runs.

  **Test scenarios:**
  - Any VMI → devices chowned to 107
  - Root VMI (RuntimeUser=0) → devices chowned to 107 (root can still access)
  - Socket permissions always set to 107
  - Network device ownerID always 107

  **Verification:**
  - `grep -r "IsNonRoot\|isNonRoot" pkg/virt-handler/ pkg/network/` → zero hits

### Phase 4: Template Service Applies Plugin LauncherPodHooks

- [ ] **Unit 4: TemplateService reads Plugin CRs and applies LauncherPodHooks**

  **Goal:** The template service evaluates matching Plugin CRs and
  strategic-merge-patches their `LauncherPodHooks` into the rendered pod
  spec. The base rendering always produces a non-root pod.

  **Requirements:** R5, R9

  **Dependencies:** Unit 1

  **Files:**
  - Modify: `pkg/virt-controller/services/template.go`
  - Modify: `pkg/virt-controller/services/rendercontainer.go`
  - Modify: `pkg/virt-controller/watch/application.go` (wire Plugin informer)

  **Approach:**
  - Wire a Plugin informer/store into the TemplateService (the informer
    exists at `virtinformers.go:887`; follow the `SidecarCreatorFunc`
    registration pattern in `application.go:715-719`).
  - Remove `IsNonRoot()` calls from `computePodSecurityContext()`,
    `renderLaunchManifest()`, `newSidecarContainerRenderer()`,
    `newInitContainerRenderer()`, `newContainerSpecRenderer()`, and
    `requiredCapabilities()`.
  - Base rendering: always set RunAsUser=107, RunAsNonRoot=true, drop ALL
    caps, add only NET_BIND_SERVICE, set AllowPrivilegeEscalation=false.
    Remove `--run-as-nonroot` entirely (handled in Unit 3).
  - After base rendering, evaluate Plugin CRs: for each Plugin with
    LauncherPodHooks, evaluate its Condition against the VMI (using the
    CEL evaluator). If matched, strategic-merge-patch the partial
    PodTemplateSpec into the rendered pod. Containers are matched by name.
  - Plugins applied in alphabetical order (same as DomainHooks).

  **Patterns to follow:**
  - `plugins/pipeline.go:88-176` — Plugin evaluation loop with CEL
    condition checking
  - `template.go:422-427` — sidecar creator loop pattern
  - `k8s.io/apimachinery/pkg/util/strategicpatch` — strategic merge patch

  **Test scenarios:**
  - VMI with no matching Plugin → non-root pod (RunAsUser=107, drop ALL)
  - VMI matching root-launcher Plugin → root pod (RunAsUser=0, SYS_NICE,
    root env vars injected)
  - Multiple Plugins with LauncherPodHooks → applied in alphabetical order
  - Plugin condition doesn't match → LauncherPodHooks not applied
  - Plugin with FailureStrategy=Ignore + CEL error → LauncherPodHooks skipped
  - Plugin adding a toleration or volume → patched into pod spec

  **Verification:**
  - `grep -r "IsNonRoot\|isNonRoot" pkg/virt-controller/services/` → zero hits
  - Pod spec rendered correctly for both cases

### Phase 5: Webhook, Migration, and Plugin CR

- [ ] **Unit 5: Simplify core vmi-mutator and migration controller**

  **Goal:** Core vmi-mutator and migration controller unconditionally set
  RuntimeUser=107. Zero awareness of root, plugins, or annotations.

  **Requirements:** R3, R4, R5

  **Dependencies:** None

  **Files:**
  - Modify: `pkg/virt-api/webhooks/mutating-webhook/mutators/vmi-mutator.go`
  - Modify: `pkg/virt-api/webhooks/mutating-webhook/mutators/vmi-mutator_test.go`
  - Modify: `pkg/virt-controller/watch/migration/migration.go`
  - Modify: `pkg/virt-controller/watch/migration/migration_test.go`

  **Approach:**
  - Core mutator: replace `if !clusterConfig.RootEnabled() { markAsNonroot() }`
    with unconditional `markAsNonroot()`. No plugin awareness, no annotation
    awareness. The mutator becomes simpler.
  - Migration controller: replace `setupVMIRuntimeUser()` with unconditional
    `RuntimeUser=107`. Remove all `RuntimeUser` comparisons, feature gate
    checks, and `DeprecatedNonRootVMIAnnotation` manipulation (callsites
    14, 26-29). The migration controller becomes simpler.
  - The plugin's mutating webhook (Unit 6) handles the RuntimeUser=0
    override and legacy upgrade path — core code does not.

  **Test scenarios:**
  - VMI without annotation → RuntimeUser=107 (core mutator only)
  - Migration always sets RuntimeUser=107 in core
  - No feature gate references remain in mutator or migration controller

  **Verification:**
  - Core mutator has zero conditionals for root/non-root
  - Migration controller has zero root-specific logic
  - `grep -rE "RootEnabled|setupVMIRuntimeUser" pkg/virt-api/ pkg/virt-controller/watch/migration/` → zero hits

- [ ] **Unit 6: Ship built-in root-launcher Plugin CR**

  **Goal:** virt-operator deploys a `root-launcher` Plugin CR with
  LauncherPodHooks for root behavior and a mutating webhook that handles
  RuntimeUser override and legacy upgrade path.

  **Requirements:** R1, R7, R8, R9

  **Dependencies:** Unit 1

  **Files:**
  - Modify: `pkg/virt-operator/resource/generate/install/`

  **Approach:**
  - Add `root-launcher` Plugin CR to operator-managed resources, gated
    behind `RootEnabled()`. When the Root feature gate is disabled
    (default), the Plugin CR is not included in the install strategy
    and not deployed. When enabled, the operator installs the Plugin.
  - The Plugin CR carries:
    1. CEL condition matching `kubevirt.io/nonroot == "false"`
    2. LauncherPodHooks: a partial `PodTemplateSpec` that patches in pod
       security context (RunAsUser=0, RunAsNonRoot=false), compute
       container security context (SYS_NICE, AllowPrivilegeEscalation=true),
       and compute container env vars for all root-specific paths and
       libvirt URI.
    3. A mutating webhook (via Plugin CRD admission references) that
       intercepts VMI CREATE and UPDATE:
       - On CREATE: if `kubevirt.io/nonroot: "false"`, set RuntimeUser=0.
       - On UPDATE (migration): if `kubevirt.io/nonroot: "false"`, ensure
         RuntimeUser=0. This preserves root across migration (R7).
       - **Upgrade path (R8):** On any CREATE/UPDATE, if RuntimeUser=0 but
         no `kubevirt.io/nonroot` annotation → add `nonroot: "false"`.
         Legacy root VMIs get auto-annotated by the plugin.
  - `PluginsEnabled()` is assumed true — no gate logic needed.

  **Test scenarios:**
  - Fresh install creates the Plugin CR
  - Plugin CR condition matches annotated VMIs
  - Plugin CR condition does not match unannotated VMIs
  - Plugin webhook sets RuntimeUser=0 on CREATE for annotated VMIs
  - Plugin webhook preserves RuntimeUser=0 on UPDATE for annotated VMIs
  - Plugin webhook auto-annotates legacy root VMIs (RuntimeUser=0, no annotation)

### Phase 6: Cleanup

- [ ] **Unit 7: Remove `IsNonRoot()`, repurpose Root feature gate**

  **Goal:** Delete remaining root/non-root branching infrastructure. Keep
  `Root` feature gate as Alpha — it now controls Plugin CR deployment.

  **Requirements:** R4, R5, R6

  **Dependencies:** Units 2, 3, 4, 5

  **Files:**
  - Delete or simplify: `pkg/vmitrait/vmitrait.go` (remove `IsNonRoot()`)
  - Modify: `staging/src/kubevirt.io/api/core/v1/types.go` (rename
    `DeprecatedNonRootVMIAnnotation` → `NonRootVMIAnnotation`)
  - Rename: `pkg/virt-handler/non-root.go` → `pkg/virt-handler/device-ownership.go`

  **Approach:**
  - Keep `Root` in `active.go` as Alpha. Its meaning changes: it controls
    whether virt-operator deploys the root-launcher Plugin CR. Update the
    comment to reflect this.
  - Keep `RootEnabled()` in `feature-gates.go` — used by the operator to
    gate Plugin CR deployment.
  - Delete `IsNonRoot()` — by this point, all callsites are already removed.
  - Rename `DeprecatedNonRootVMIAnnotation` to `NonRootVMIAnnotation`.
    The annotation is revived with value semantics, not removed.
  - Rename `non-root.go` since it's no longer about "non-root" — it handles
    device ownership generically.

  **Verification:**
  - `grep -rE "IsNonRoot|isNonRoot|DeprecatedNonRootVMI|run-as-nonroot" pkg/ cmd/ staging/` → zero hits
  - `Root` remains in `active.go` as Alpha
  - `RootEnabled()` exists and is used only by operator Plugin CR deployment
  - Codebase compiles

- [ ] **Unit 8: Update tests**

  **Goal:** Update all tests that reference the Root feature gate or
  `IsNonRoot()`.

  **Requirements:** R1-R9

  **Dependencies:** Units 1-7

  **Files:**
  - Modify: `tests/migration/migration.go`
  - Modify: `tests/security_features_test.go`
  - Modify: `tests/storage/hostdisk.go`
  - Modify: `tests/testsuite/namespace.go`
  - Modify: `tests/libdomain/domain.go`
  - Modify: `pkg/vmitrait/vmitrait_test.go`
  - Modify: `pkg/virt-controller/services/template_test.go`
  - Create: e2e tests for Plugin-based root launcher

  **Approach:**
  - Replace all feature gate toggling with annotation-based tests.
  - Test both paths via presence/absence of annotation + Plugin CR.
  - Verify env vars appear in rendered pod spec when Plugin matches.
  - Delete the 7 "migration to nonroot" / "migration to root" test entries
    (`test_id:8609-8612` + 3 more) — the scenario of switching UID via
    feature gate toggle no longer exists.
  - Replace with a legacy upgrade path test (R8): create a VMI with
    RuntimeUser=0 but no `nonroot` annotation (simulating a pre-refactor
    root VMI), migrate it, verify the annotation `nonroot: "false"` is
    auto-added and the VMI continues running as root on the destination.

  **Verification:**
  - No test references `IsNonRoot()`
  - Tests that need root-via-Plugin enable the `Root` feature gate to
    deploy the Plugin CR, then use annotation-based VMI creation
  - New e2e: legacy root VMI (RuntimeUser=0, no annotation) → migrate →
    `nonroot: "false"` auto-added, still runs as UID 0

## System-Wide Impact

- **Interaction graph:** The TemplateService gains Plugin CRD awareness
  (generic LauncherPodHooks, not root-specific). The core mutating webhook
  and migration controller become simpler — unconditional RuntimeUser=107.
  virt-handler and virt-launcher become config-driven. All root-specific
  logic lives entirely in the plugin.

- **Error propagation:** PSA rejects the pod if namespace isn't privileged.
  Existing handler in `lifecycle.go:175-178` surfaces a helpful message.

- **No mixed-mode risk:** Each component reads its own config (env vars,
  RuntimeUser UID) per-VMI. Two VMIs on the same node — one root, one
  non-root — work correctly because their pods have different env vars
  and security contexts.

- **State lifecycle:** Existing root VMIs keep RuntimeUser=0 after upgrade.
  On migration, the plugin's mutating webhook detects legacy root VMIs
  (RuntimeUser=0, no annotation) and automatically adds
  `nonroot: "false"`, so they continue as root on the destination.

- **API surface:** Existing `kubevirt.io/nonroot` annotation gains value
  semantics (`"false"` = root). Plugin CRD gains `LauncherPodHooks` — a
  generic `PodTemplateSpec` patch usable by any plugin author. `virtctl`
  needs no changes.

## Risks & Dependencies

- **Scope (medium):** This is a deeper refactor than just moving the decision
  point. 8 units across 6 phases. Phases 2-4 can run in parallel.

- **Upgrade risk (low):** The plugin's mutating webhook automatically
  annotates legacy root VMIs with `nonroot: "false"` on migration. No
  manual intervention needed, assuming the root-launcher Plugin is installed.

- **Plugin feature gate coupling (none):** `PluginsEnabled()` is assumed true.
  The entire plan depends on this precondition.

- **Root feature gate semantic shift (low):** The `Root` gate changes meaning
  from "all VMs run as root" to "deploy the root-launcher Plugin CR."
  Clusters that had Root enabled will get the Plugin deployed, preserving
  root capability. Clusters without Root see no change.

- **CEL evaluator in webhook context (low):** Need to verify it handles
  nil domain for condition-only evaluation.

- **Env var naming (low):** Choosing stable env var names is an API
  decision. Names in this plan are directional.

## Documentation / Operational Notes

- Release notes: Root feature gate repurposed — now controls deployment
  of the root-launcher Plugin CR. Per-VMI root via annotation.
- Upgrade guide: clusters with Root enabled get the Plugin deployed
  automatically. Plugin webhook auto-annotates legacy root VMIs on
  migration.
- User guide: root-launcher Plugin usage, namespace PSA requirement.
- Developer guide: LauncherPodHooks — how plugin authors can override any
  virt-launcher pod spec field via strategic merge patch.

## Sources & References

- Plugin CRD types: `staging/src/kubevirt.io/api/plugin/v1alpha1/types.go`
- Plugin pipeline: `pkg/virt-launcher/virtwrap/plugins/pipeline.go`
- Plugin CEL evaluator: `pkg/virt-launcher/virtwrap/plugins/cel/evaluator.go`
- Plugin informer: `pkg/controller/virtinformers.go:887`
- Feature gate: `pkg/virt-config/featuregate/active.go:44,283`
- Discontinuation pattern: `pkg/virt-config/featuregate/inactive.go:260`
- VMI mutator: `pkg/virt-api/webhooks/mutating-webhook/mutators/vmi-mutator.go:86-88`
- Migration RuntimeUser: `pkg/virt-controller/watch/migration/migration.go:1941-1968`
- IsNonRoot: `pkg/vmitrait/vmitrait.go:26-30`
- Pod security context: `pkg/virt-controller/services/template.go:360-379`
- Container capabilities: `pkg/virt-controller/services/rendercontainer.go:292-302`
- Device ownership: `pkg/virt-handler/non-root.go:192-207`
- Reactive PSA handling: `pkg/virt-controller/watch/vmi/lifecycle.go:175-178`
- virt-launcher flags: `cmd/virt-launcher/virt-launcher.go:357`

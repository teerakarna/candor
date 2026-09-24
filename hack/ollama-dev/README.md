# Ollama dev sandbox

A hardened local Ollama instance for developing/testing the Ollama LLM backend
([issue #52](https://github.com/teerakarna/candor/issues/52)) without running an unsandboxed
process directly on your host with your own user's full privileges.

Dev tool only: not part of the release build, not shipped anywhere, no Go package imports it -
same posture as [`hack/grafana-preview/`](../grafana-preview/).

## Run it

```sh
cd hack/ollama-dev
docker compose up -d   # or: podman compose up -d
```

Pull a model and test it, same as any Ollama install:

```sh
docker compose exec ollama ollama pull llama3.2:3b
curl http://localhost:11434/api/version
```

```sh
docker compose down    # add -v to also drop the model volume
```

## What's actually hardened, and why

| Control | What | Why |
|---|---|---|
| Image | pinned by digest, not a floating tag | same discipline as every other image reference in this repo |
| User | forced `1000:1000` | the image defaults to root - nothing in its own config sets a `USER` |
| Root filesystem | read-only, `tmpfs` for `/tmp` only (`noexec,nosuid`) | a compromised process can't persist anything to the image layer |
| Capabilities | all dropped, none added | serving HTTP and running inference needs zero special capabilities |
| Privilege escalation | blocked (`no-new-privileges`) | closes the setuid-binary escalation path even as non-root |
| Process count | capped at 256 | bounds a runaway fork loop |
| Memory / CPU | capped at 4GB / 4 cores | bounds resource exhaustion from a single model load or a flood of requests |
| Network | published to `127.0.0.1` only | not reachable from your LAN - change deliberately, not by accident |
| Model storage | a dedicated named volume | no host directory bind-mounted in |

If you're on Podman rather than Docker Desktop, you also get a real hypervisor boundary underneath
all of the above for free - Podman on macOS runs containers inside its own Linux VM (`applehv`),
not directly on the host, and rootless mode means there's no privileged daemon either.

## Two non-obvious gotchas, hit and fixed while building this

**Wrong home directory silently fails.** The image's `/etc/passwd` maps UID 1000 to `ubuntu`, home
`/home/ubuntu` - not `/root`. Mounting the model volume at `/root/.ollama` while running as
`1000:1000` fails with `mkdir /home/ubuntu/.ollama: read-only file system`, which reads like a
permissions bug but is actually just the wrong path. Check `/etc/passwd` in the image directly
before assuming a path (`docker run --rm --entrypoint cat <image> /etc/passwd`) rather than
guessing - this changed between an earlier version of this image and 0.13.0.

**A container memory limit is a no-op above your VM's own ceiling.** Docker Desktop and Podman
Machine both run containers inside a Linux VM with its own fixed memory allocation, independent of
`mem_limit`/`--memory`. A VM capped at 2GB will still let a 4GB container limit through, then
silently kill the model-loading process once real usage exceeds what the VM actually has - the
error surfaces as `"llama runner process has terminated: signal: killed"`, not an OOM message that
points at the VM. Check your VM's own memory first (`podman machine inspect --format
'{{.Resources.Memory}}'`, or Docker Desktop's Resources settings) before assuming the container
flag is being respected. A small model (3-8B, quantized) comfortably needs 4-6GB of *VM* memory,
not just container memory.

## What this doesn't cover

CPU-only inference - neither Docker Desktop nor Podman Machine passes a GPU through to the Linux VM
on macOS, so expect CPU-speed generation, fine for interactive testing, not a production
performance signal. No model-management automation is provided here on purpose - issue #52
explicitly excludes bundling that; this sandbox is for developing against Ollama, not for how
Candor itself would deploy it (that's its own pod/container, unrelated to the operator's own
ServiceAccount or RBAC).

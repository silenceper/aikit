# Security Policy

aikit manages executable instructions, Git sources, local configuration, and
filesystem links across developer workspaces. Please report security issues
privately so maintainers have time to investigate and coordinate a fix.

## Supported versions

| Version | Security fixes |
|---|---|
| Current `main` branch | Yes, on a best-effort basis |
| Latest published alpha | Yes, when a fix can be released safely |
| Older alpha releases | No guaranteed backports |

The project is currently alpha software. Until a stable release policy is
published, users should upgrade to the latest release after reviewing its
changelog and keep recoverable backups of important configuration.

## Reporting a vulnerability

Do **not** open a public issue for a suspected vulnerability. Email
`silenceper@gmail.com` with the subject `[aikit security] <short summary>`.

Include, when available:

- affected aikit version or commit;
- operating system and filesystem;
- impact and the security boundary that was crossed;
- minimal reproduction steps or a proof of concept;
- whether the issue requires a concurrent local process, crafted repository,
  malicious symlink, credentials, or a specific configuration;
- suggested mitigations or fixes.

Remove real credentials, private repository URLs, personal paths, and unrelated
user data. If sensitive material is required to reproduce the issue, first ask
for a safe transfer method.

The maintainers aim to acknowledge a report within five business days and send
an initial assessment within ten business days. These are best-effort targets,
not a service-level agreement. Please allow coordinated disclosure until a fix
and release plan are available.

## Security-relevant areas

Reports are especially useful for:

- escaping a configured library or workspace root;
- overwriting or deleting content that aikit cannot authenticate as managed;
- symlink, reparse-point, path-replacement, or file-identity races;
- bypassing mutation serialization or recovery gates;
- leaking Git credentials, authorization headers, or source secrets;
- unsafe update transport changes or cache poisoning;
- preview, dry-run, status, or offline operations that unexpectedly mutate
  files or access the network.

General bugs, feature requests, and support questions should use the channels
described in [SUPPORT.md](SUPPORT.md).

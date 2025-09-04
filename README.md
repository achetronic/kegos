# KEGOS (Keycloak Google Workspace Groups Syncer)

![GitHub go.mod Go version (subdirectory of monorepo)](https://img.shields.io/github/go-mod/go-version/achetronic/kegos)
![GitHub](https://img.shields.io/github/license/achetronic/kegos)

![YouTube Channel Subscribers](https://img.shields.io/youtube/channel/subscribers/UCeSb3yfsPNNVr13YsYNvCAw?label=achetronic&link=http%3A%2F%2Fyoutube.com%2Fachetronic)
![GitHub followers](https://img.shields.io/github/followers/achetronic?label=achetronic&link=http%3A%2F%2Fgithub.com%2Fachetronic)
![X (formerly Twitter) Follow](https://img.shields.io/twitter/follow/achetronic?style=flat&logo=twitter&link=https%3A%2F%2Ftwitter.com%2Fachetronic)

A daemon process to automatically synchronize Google Workspace (GSuite) group memberships for users present in Keycloak, following an authoritative approach.

## Motivation

While Keycloak can authenticate users against Google Workspace via OIDC, it doesn't automatically sync Google Workspace group memberships. Users may belong to various groups in Google Workspace that define their permissions and roles, but these group memberships are not reflected in Keycloak by default.

This synchronizer bridges that gap by continuously monitoring users in Keycloak and automatically syncing their Google Workspace group memberships. As users authenticate to Keycloak, their Google Workspace groups are automatically added to their Keycloak profile, enabling proper authorization and role-based access control.

## How it works

1. **Discovery**: KEGOS retrieves all users from the specified Keycloak realm
2. **Group Resolution**: For each user, it queries Google Workspace Admin API to get their group memberships
3. **Synchronization**: Groups are created in Keycloak if they don't exist, and users are added/removed from groups to match Google Workspace
4. **Continuous Sync**: The process repeats at configurable intervals to keep memberships up-to-date

## Flags

Every configuration parameter can be defined by flags that can be passed to the CLI.
They are described in the following table:

| Name                       | Description                                               | Default | Example                                            |
|:---------------------------|:----------------------------------------------------------|:--------|----------------------------------------------------|
| `--log-level`              | Define the verbosity of the logs                          | `info`  | `--log-level debug`                                |
| `--gsuite-credentials`     | Path to Google Workspace service account credentials JSON | -       | `--gsuite-credentials="/path/to/credentials.json"` |
| `--gsuite-domain`          | Google Workspace domain to sync groups from               | -       | `--gsuite-domain="company.com"`                    |
| `--keycloak-uri`           | Keycloak server URI                                       | -       | `--keycloak-uri="https://auth.company.com"`        |
| `--keycloak-realm`         | Keycloak realm to sync users and groups                   | -       | `--keycloak-realm="master"`                        |
| `--keycloak-client-id`     | Keycloak client ID with admin permissions                 | -       | `--keycloak-client-id="kegos"`                      |
| `--keycloak-client-secret` | Keycloak client secret                                    | -       | `--keycloak-client-secret="super-secret"`          |
| `--reconcile-interval`     | Time between synchronization cycles (duration format)     | `10m`   | `--reconcile-interval="5m"`                        |
| `--synced-parent-group`    | Keycloak group where to sync Gsuite groups                | -       | `--synced-parent-group="google-workspace"`         |
| `--help`                   | Show help information                                     | `false` | `--help`                                           |

## Prerequisites

### Google Workspace Setup

1. Create a service account in Google Cloud Console
2. Enable the Admin SDK API
3. Download the service account JSON credentials
4. Grant domain-wide delegation to the service account
5. In Google Admin Console, authorize the service account with the following OAuth scopes:
- `https://www.googleapis.com/auth/admin.directory.group.readonly`
- `https://www.googleapis.com/auth/admin.directory.user.readonly`

### Keycloak Setup

1. Create a client in Keycloak with service account enabled
2. Assign the following client roles from `realm-management`:
- `manage-users` (to manage user group memberships)
- `view-users` (to read user information)
- `manage-realm` (to create and manage groups)

## Examples

### Using Command-line Flags

Here you have a complete example to use this command with flags:

```console
kegos \
 --log-level=info \
 --gsuite-credentials="/opt/kegos/gsuite-credentials.json" \
 --gsuite-domain="freepik.com" \
 --keycloak-uri="https://keycloak.freepik.com" \
 --keycloak-realm="employees" \
 --keycloak-client-id="kegos-sync" \
 --keycloak-client-secret="your-client-secret" \
 --reconcile-interval="15m" \
 --synced-parent-group="google-workspace"
```

### Using Environment Variables

You can also mix both approaches, with environment variables:

```console
export GSUITE_CREDENTIALS="/opt/kegos/gsuite-credentials.json"
export GSUITE_DOMAIN="freepik.com"
export KEYCLOAK_URI="https://keycloak.freepik.com"
export KEYCLOAK_REALM="employees"
export KEYCLOAK_CLIENT_ID="kegos-sync"
export KEYCLOAK_CLIENT_SECRET="your-client-secret"
export SYNCED_PARENT_GROUP="google-workspace"

kegos --log-level=info --reconcile-interval="15m"
```

## How to use

This project provides binary files and Docker images to make it easy to use wherever wanted.

### Binaries

Binary files for the most popular platforms will be added to the [releases](https://github.com/achetronic/kegos/releases)

### Docker

Docker images can be found in GitHub's [packages](https://github.com/achetronic/kegos/pkgs/container/kegos)
related to this repository.

```bash
docker run --rm \
 -v /path/to/gsuite-creds.json:/credentials.json:ro \
 ghcr.io/achetronic/kegos:latest \
 --gsuite-credentials="/credentials.json" \
 --gsuite-domain="your-domain.com" \
 --keycloak-uri="https://your-keycloak.com" \
 --keycloak-realm="your-realm" \
 --keycloak-client-id="your-client" \
 --keycloak-client-secret="your-secret" \
 --synced-parent-group="google-workspace"
```

## Security Considerations

- Store the Keycloak client secret securely (consider using environment variables or secret management)
- Limit the Google Workspace service account permissions to read-only
- Use a dedicated Keycloak client with minimal required permissions
- Monitor logs for authentication failures or API rate limits

## How to contribute

We are open to external collaborations for this project: improvements, bugfixes, whatever.

For doing it, open an issue to discuss the need of the changes, then:

- Fork the repository
- Make your changes to the code
- Open a PR and wait for review

The code will be reviewed and tested (always)

> We are developers and hate bad code. For that reason we ask you the highest quality
> on each line of code to improve this project on each iteration.

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

## Special mention

This project was done using IDEs from JetBrains. They helped us to develop faster, so we recommend them a lot! 🤓

<img src="https://resources.jetbrains.com/storage/products/company/brand/logos/jb_beam.png" alt="JetBrains Logo (Main) logo." width="150">


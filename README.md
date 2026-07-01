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
2. **Group Resolution**: For each user, it derives the Google Workspace domain from the user's email and queries the Admin API for that user's group memberships in that domain
3. **Synchronization**: Groups are created in Keycloak if they don't exist, and users are added/removed from groups to match Google Workspace
4. **Continuous Sync**: The process repeats at configurable intervals to keep memberships up-to-date

The set of users to sync is whatever exists in the Keycloak realm: if a user managed to log into Keycloak, its domain is trusted. Each user only receives groups from its own domain (the part after the `@`). When a user logs in through a domain alias, enable `--resolve-aliases` so KEGOS resolves the Google primary email first and matches the real domain.

## Flags

Every configuration parameter can be defined by flags that can be passed to the CLI.
They are described in the following table:

| Name                       | Description                                                               | Default | Example                                            |
| :------------------------- | :------------------------------------------------------------------------ | :------ | -------------------------------------------------- |
| `--log-level`              | Define the verbosity of the logs                                          | `info`  | `--log-level debug`                                |
| `--gsuite-credentials`     | Path to Google Workspace service account credentials JSON                 | -       | `--gsuite-credentials="/path/to/credentials.json"` |
| `--resolve-aliases`        | Resolve each Keycloak username to its Google primary email before syncing | `false` | `--resolve-aliases`                                |
| `--user-rate-limit`        | Max users processed per minute against the Google API (0 disables it)     | `60`    | `--user-rate-limit=120`                            |
| `--keycloak-uri`           | Keycloak server URI                                                       | -       | `--keycloak-uri="https://auth.company.com"`        |
| `--keycloak-realm`         | Keycloak realm to sync users and groups                                   | -       | `--keycloak-realm="master"`                        |
| `--keycloak-client-id`     | Keycloak client ID with admin permissions                                 | -       | `--keycloak-client-id="kegos"`                     |
| `--keycloak-client-secret` | Keycloak client secret                                                    | -       | `--keycloak-client-secret="super-secret"`          |
| `--reconcile-interval`     | Time between synchronization cycles (duration format)                     | `10m`   | `--reconcile-interval="5m"`                        |
| `--synced-parent-group`    | Keycloak group where to sync Gsuite groups                                | -       | `--synced-parent-group="google-workspace"`         |
| `--help`                   | Show help information                                                     | `false` | `--help`                                           |

## Prerequisites

### Google Workspace Setup

KEGOS talks to the Admin SDK Directory API using a Delegated Admin Service Account (DASA):
a regular GCP service account that is granted a Workspace delegated admin role directly.

Ref: https://github.com/GAM-team/GAM/wiki/Using-GAM7-with-a-delegated-admin-service-account
Ref: https://knowledge.workspace.google.com/admin/users/assign-specific-admin-roles

#### 1. Google Cloud (gcloud)

```bash
PROJECT_ID="your-project"

# Enable the Admin SDK API in the service account's project
gcloud services enable admin.googleapis.com --project="$PROJECT_ID"

# Create the service account
gcloud iam service-accounts create kegos \
  --display-name="kegos directory reader" \
  --project="$PROJECT_ID"

SA_EMAIL="kegos@${PROJECT_ID}.iam.gserviceaccount.com"

# Create the key that KEGOS reads via --gsuite-credentials
gcloud iam service-accounts keys create gsuite-credentials.json \
  --iam-account="$SA_EMAIL"

# Print the email and numeric unique ID (needed in the Admin console)
gcloud iam service-accounts describe "$SA_EMAIL" \
  --format="value(email, uniqueId)"
```

No project-level IAM roles are needed to read the directory: read access is granted in
Workspace, not in Cloud IAM.

#### 2. Google Workspace Admin console

You need to be a super admin. Menus live at https://admin.google.com.

1. Create a delegated admin role with read-only access:
   `Account` > `Admin roles` > `Create new role`. Under Admin API privileges grant only:
   - `Users` > `Read`
   - `Groups` > `Read`

2. Assign the service account to that role:
   open the role > `Assign service accounts` > enter the service account email from step 1.

3. The scopes (`admin.directory.user.readonly`, `admin.directory.group.readonly`) are
   requested by KEGOS itself; nothing to configure for them in the console.

Role assignments can take a few minutes to propagate.

Ref: https://support.google.com/a/answer/33325

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
 --resolve-aliases \
 --user-rate-limit=120 \
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
 --resolve-aliases \
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

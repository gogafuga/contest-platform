# Infrastructure history

## Yandex Cloud deployment

The contest ran on a Yandex Compute Cloud VM named `contest01` in
`ru-central1-a`. It ran Ubuntu 24.04 with two Ice Lake vCPUs at 50%,
2 GB RAM, and a 10 GB SSD. Its reserved public IP was `46.21.244.208`,
and it served the app over HTTP on port 80.

Source was transferred with rsync. `docker-compose.cloud.yml` ran the bundled
UI/API and private PostgreSQL with persistent storage, restart policies, and
bounded logs. Secrets lived in `~/contest01/.env.cloud` on the VM. SSH required
keys, and direct root login was disabled.

After deployment, local API integration tests and public HTTP smoke checks
passed. The smoke checks covered registration, task selection and dataset
downloads, all three submissions with extra empty lines, and the 375-point total.

## Teardown

On 2026-11-22, the app was stopped and PostgreSQL was dumped to the private,
gitignored local file `.contest/contest01-cloud-TwUqWW.sql`. The dump completed
successfully. A restore was not tested.

The VM, boot disk, reserved public IP, and dedicated `contest01-web` security
group were deleted and their removal verified. The folder's default network,
subnets, and security group were retained.

The cloud Compose configuration remains available for future deployments.
See the [deployment runbook](README.md) for setup and backup commands.

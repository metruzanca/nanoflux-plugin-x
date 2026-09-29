# nanoflux-plugin-x

A [nanoflux](https://github.com/metruzanca/nanoflux)↗ external plugin for X
(Twitter) profile feeds.

X hides posts from logged-out visitors. This plugin reads a profile with your
own logged-in session, so those posts show up as a normal feed in nanoflux.

> **Use at your own risk.** This plugin logs in with your account's cookies and
> reads the page the same way a browser does. X does not offer this as an API,
> so it can break or get your session limited at any time. Keep your cookies
> private and only run this for your own account.

## Install

Clone this repo next to a nanoflux checkout and build the binary into its
`plugins/` directory:

```bash
git clone <this-repo> nanoflux/plugins/x
cd nanoflux/plugins/x
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ../nanoflux-plugin-x .
```

The binary lands in `nanoflux/plugins/`, which is where nanoflux looks for
plugins (`NF_PLUGINS_DIR`, `/plugins` in the container). Restart nanoflux; the
log should show `registered external plugin x (api 0.2)`.

Adjust `GOARCH` if your host is not `amd64` (use `arm64` for ARM).

## Configure your session

The plugin needs two cookies from a logged-in x.com session. Set them in
nanoflux, not in a file: an admin opens **/admin**, finds the `x` plugin card,
and fills in its settings.

1. Open x.com in your browser and log in.
2. Open DevTools → **Application** → **Cookies** → `https://x.com`.
3. Copy the `auth_token` and `ct0` cookie values into the plugin's settings on
   the admin page and save.

- `auth_token` is your login session.
- `ct0` is a security token X also expects.

If posts stop showing up, log out/in, copy the cookies again, and update the
settings — sessions expire.

## Usage

Add a feed with a profile URL:

```
https://x.com/<handle>
```

## Notes

- Only recent posts are fetched. There is no "load older items" paging.
- X changes its page often. When it does, the plugin needs an update; it never
  creates duplicate items, it just stops finding new ones.
- If X rate-limits you, nanoflux waits and retries automatically, like any feed.

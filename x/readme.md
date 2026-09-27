# x

X hides posts from logged-out visitors and hard-walls sensitive accounts. This
plugin reads a profile with your own logged-in session, so those posts show up
as a normal feed.

> **Use at your own risk.** It logs in with your account's cookies and reads the
> page the way a browser does. X does not offer this as an API, so it can break
> or get your session limited at any time. Keep your cookies private and only
> run it for your own account.

## Quirks

- **It needs a session.** The plugin reads `auth_token`/`ct0` cookies from its
  session config; without a valid session a profile cannot be read.
- **Only recent posts are fetched.** There is no "load older items" paging.
- **It is fragile by design.** X changes its page often; when it does the plugin
  needs an update. It does not create duplicate items when that happens, it
  simply stops finding new ones.
- **A tweet's id is its identity**, so a link-shape change cannot store the same
  post twice.

## Filtering

Items carry the post text, so the `title` and `summary` filter fields work as
usual. There are no site-specific categories.

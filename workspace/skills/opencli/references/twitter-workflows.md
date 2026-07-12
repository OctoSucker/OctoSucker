# X/Twitter Operation Selection

Choose operations by the requested object:

- `timeline`: the logged-in user's home feed.
- `tweets`: recent posts by one user, or by the logged-in user when supported and omitted.
- `search`: posts matching a query.
- `thread`: one status plus its replies; accepts the input form defined by the tool schema.
- `article`: X long-form article content.
- `profile`: account metadata.
- `following`: accounts followed by a user.
- `bookmarks`: the logged-in user's saved posts.
- `lists` and `list-tweets`: list discovery and list feed retrieval.
- `notifications`: the logged-in user's notifications.
- `trending`: current trending topics.

Do not replace one operation with another just because both return tweets. In particular, timeline is not a user's posting history, and article is not a normal thread.

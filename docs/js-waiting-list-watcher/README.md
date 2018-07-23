# JS Waiting List Watcher

Example javascript page to show the current watching list of a given service,
when that service exposes the websockets api endpoint.

The dcrstmd service needs to be run with the `WaitingListWSBindAddr` specified
to a public address/port (or better yet, run an nginx proxy in front of it).

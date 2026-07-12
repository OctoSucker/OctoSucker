# Browser Authentication

OpenCLI browser adapters reuse an existing Chrome login through the OpenCLI Browser Bridge.

Required state:

1. Chrome is running.
2. The user is already logged into the target website in Chrome.
3. The OpenCLI Browser Bridge extension is installed and reachable.

There is no OpenCLI command that performs an X/Twitter login. If a read tool reports an authentication or bridge error, explain the concrete prerequisite and stop. Do not retry with invented commands or request cookies, passwords, session tokens, or other credentials.

Login verification should use a harmless read operation already exposed in the tool catalog. A successful structured response confirms access; an empty feed does not by itself prove authentication failure.

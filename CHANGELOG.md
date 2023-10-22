# CHANGELOG

## v1.17.3 (22nd October 2023)

- Fix UI bug in the DNS page causing the user unable to modify domains

## v1.17.2 (22nd October 2023)

- Add support for running on Docker

## v1.17.1 (22nd October 2023)

- Fix: DNS Server blocklist in default settings

## v1.17 (22nd October 2023)

- Fix: DNS Server not updating list immediately after an update from the UI
- Added link to download certificate on the login screen
- Added DNS server info page which shows total blocked domains, last update time and next scheduled update for blocklists
- Added filtering strictness field to the UI
- Fixed bug : Missing mapping of exception hosts in the Web UI
- Fixed bug : AVIF files were not being properly displayed in Firefox
- Refactored code
- Now the content type filter also blocks content immediately by guessing the file type from the URL, this can be helpful in terms of saving bandwidth. Previously, we would send the request to the server and block content based upon the response MIME type
- Added tests for basic functionality and code coverage
- Fixed blocked responses for some cases, where if we detected blocked content we would simply terminate the connection, now we send a proper blocked page
- Added support for sending an image with the text BLOCKED for blocked images.
- Introduced the CHANGELOG

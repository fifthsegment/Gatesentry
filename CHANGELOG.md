# CHANGELOG

## v1.17 (21st October 2023)

- Fixed bug : Missing mapping of exception hosts in the Web UI
- Fixed bug : AVIF files were not being properly displayed in Firefox
- Refactored code
- Now the content type filter also blocks content immediately by guessing the file type from the URL, this can be helpful in terms of saving bandwidth. Previously, we would send the request to the server and block content based upon the response MIME type
- Added tests for basic functionality and code coverage
- Fixed blocked responses for some cases, where if we detected blocked content we would simply terminate the connection, now we send a proper blocked page
- Added support for sending an image with the text BLOCKED for blocked images.
- Introduced the CHANGELOG

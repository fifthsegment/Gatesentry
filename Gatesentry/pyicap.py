"""Implements an ICAP server framework

For the ICAP specification, see RFC 3507
"""

__version__ = "1.0"

__all__ = ['ICAPServer', 'BaseICAPRequestHandler', 'ICAPError']

import sys
import time
import random
import socket
import urlparse
import SocketServer

class ICAPError(Exception):
    """Signals a protocol error"""
    def __init__(self, code=500, message=None):
        if message == None:
            message = BaseICAPRequestHandler._responses[code]

        super(ICAPError, self).__init__(message)
        self.code = code

class ICAPServer(SocketServer.TCPServer):
    """ICAP Server

    This is a simple TCPServer, that allows address reuse
    """
    allow_reuse_address = 1

class BaseICAPRequestHandler(SocketServer.StreamRequestHandler):
    """ICAP request handler base class.

    You have to subclass it and provide methods for each service
    endpoint. Every endpoint MUST have an _OPTION method, and either
    a _REQMOD or a _RESPMOD method.
    """

    # The version of the ICAP protocol we support.
    protocol_version = "ICAP/1.0"

    # Table mapping response codes to messages; entries have the
    # form {code: (shortmessage, longmessage)}.
    # See RFC 2616 and RFC 3507
    _responses = {
        100: ('Continue', 'Request received, please continue'),
        101: ('Switching Protocols',
              'Switching to new protocol; obey Upgrade header'),

        200: ('OK', 'Request fulfilled, document follows'),
        201: ('Created', 'Document created, URL follows'),
        202: ('Accepted',
              'Request accepted, processing continues off-line'),
        203: ('Non-Authoritative Information', 'Request fulfilled from cache'),
        204: ('No Content', 'Request fulfilled, nothing follows'),
        205: ('Reset Content', 'Clear input form for further input.'),
        206: ('Partial Content', 'Partial content follows.'),

        300: ('Multiple Choices',
              'Object has several resources -- see URI list'),
        301: ('Moved Permanently', 'Object moved permanently -- see URI list'),
        302: ('Found', 'Object moved temporarily -- see URI list'),
        303: ('See Other', 'Object moved -- see Method and URL list'),
        304: ('Not Modified',
              'Document has not changed since given time'),
        305: ('Use Proxy',
              'You must use proxy specified in Location to access this '
              'resource.'),
        307: ('Temporary Redirect',
              'Object moved temporarily -- see URI list'),

        400: ('Bad Request',
              'Bad request syntax or unsupported method'),
        401: ('Unauthorized',
              'No permission -- see authorization schemes'),
        402: ('Payment Required',
              'No payment -- see charging schemes'),
        403: ('Forbidden',
              'Request forbidden -- authorization will not help'),
        404: ('Not Found', 'Nothing matches the given URI'),
        405: ('Method Not Allowed',
              'Specified method is invalid for this resource.'),
        406: ('Not Acceptable', 'URI not available in preferred format.'),
        407: ('Proxy Authentication Required', 'You must authenticate with '
              'this proxy before proceeding.'),
        408: ('Request Timeout', 'Request timed out; try again later.'),
        409: ('Conflict', 'Request conflict.'),
        410: ('Gone',
              'URI no longer exists and has been permanently removed.'),
        411: ('Length Required', 'Client must specify Content-Length.'),
        412: ('Precondition Failed', 'Precondition in headers is false.'),
        413: ('Request Entity Too Large', 'Entity is too large.'),
        414: ('Request-URI Too Long', 'URI is too long.'),
        415: ('Unsupported Media Type', 'Entity body in unsupported format.'),
        416: ('Requested Range Not Satisfiable',
              'Cannot satisfy request range.'),
        417: ('Expectation Failed',
              'Expect condition could not be satisfied.'),

        500: ('Internal Server Error', 'Server got itself in trouble'),
        501: ('Not Implemented',
              'Server does not support this operation'),
        502: ('Bad Gateway', 'Invalid responses from another server/proxy.'),
        503: ('Service Unavailable',
              'The server cannot process the request due to a high load'),
        504: ('Gateway Timeout',
              'The gateway server did not receive a timely response'),
        505: ('Protocol Version Not Supported', 'Cannot fulfill request.'),

    }

    # The Python system version, truncated to its first component.
    _sys_version = "Python/" + sys.version.split()[0]

    # The server software version.  You may want to override this.
    # The format is multiple whitespace-separated strings,
    # where each string is of the form name[/version].
    _server_version = "BaseICAP/" + __version__

    _weekdayname = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']

    _monthname = [None,
                 'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
                 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

    def _read_status(self):
        """Read a HTTP or ICAP status line from input stream"""
        return self.rfile.readline().strip().split(' ', 2)

    def _read_request(self):
        """Read a HTTP or ICAP request line from input stream"""
        return self.rfile.readline().strip().split(' ', 2)

    def _read_headers(self):
        """Read a sequence of header lines"""
        headers = {}
        while True:
            line = self.rfile.readline().strip()
            if line == '':
                break
            k, v = line.split(':', 1)
            headers[k.lower()] = headers.get(k.lower(), []) + [v.strip()]
        return headers

    def read_chunk(self):
        """Read a HTTP chunk

        Also handles the ieof chunk extension defined by the ICAP
        protocol by setting the ieof variable to True. It returns an
        empty line if the last chunk is read. Reading after the last
        chunks will return empty strings.
        """

        # Don't try to read when there's no body
        if not self.has_body or self.eob:
            self.eob = True
            return ''

        line = self.rfile.readline()
        if line == '':
            # Connection was probably closed
            self.eob = True
            return ''

        line = line.strip()

        arr = line.split(';', 1)

        chunk_size = 0
        try:
            chunk_size = int(arr[0], 16)
        except ValueError:
            raise ICAPError(400, 'Protocol error, could not read chunk')

        # Look for ieof chunk extension
        if len(arr) > 1 and arr[1].strip() == 'ieof':
            self.ieof = True

        value = self.rfile.read(chunk_size)
        self.rfile.read(2)

        if value == '':
            self.eob = True

        return value

    def write_chunk(self, data):
        """Write a chunk of data

        When finished writing, an empty chunk with data='' must
        be written.
        """
        l = hex(len(data))[2:]
        self.wfile.write(l + '\r\n' + data + '\r\n')

    def cont(self):
        """Send a 100 continue reply

        Useful when the client sends a preview request, and we have
        to read the entire message body. After this command, read_chunk
        can safely be called again.
        """
        if self.ieof:
            raise ICAPError(500, 'Tried to continue on ieof condition')

        self.wfile.write('ICAP/1.0 100 Continue\r\n\r\n')

        self.eob = False

    def set_enc_status(self, status):
        """Set encapsulated status in response

        ICAP responses can only contain one encapsulated header section.
        Such section is either an encapsulated HTTP request, or a
        response. This method can be called to set encapsulated HTTP
        response's status line.
        """
        # TODO: some semantic checking might be OK
        self.enc_status = status

    def set_enc_request(self, request):
        """Set encapsulated request line in response

        ICAP responses can only contain one encapsulated header section.
        Such section is either an encapsulated HTTP request, or a
        response. This method can be called to set encapsulated HTTP
        request's request line.
        """
        # TODO: some semantic checking might be OK
        self.enc_request = request

    # TODO: write add_* and set_* methods
    # TODO: also add convenient mode to query these
    def set_enc_header(self, header, value):
        """Set an encapsulated header to the given value

        Multiple sets will cause the header to be sent multiple times.
        """
        self.enc_headers[header] = self.enc_headers.get(header, []) + [value]

    def set_icap_response(self, code):
        """Sets the ICAP response's status line and response code"""
        self.icap_response = 'ICAP/1.0 ' + str(code) + ' ' + self._responses[code][0]
        self.icap_response_code = code

    def set_icap_header(self, header, value):
        """Set an ICAP header to the given value

        Multiple sets will cause the header to be sent multiple times.
        """
        self.icap_headers[header] = self.icap_headers.get(header, []) + [value]

    def send_headers(self, has_body = False):
        """Send ICAP and encapsulated headers

        Assembles the Encapsulated header, so it's need the information
        of wether an encapsulated message body is present.
        """
        enc_header = None
        enc_req_stat = ''
        if self.enc_request != None:
            enc_header = 'req-hdr=0'
            enc_body = 'req-body='
            enc_req_stat = self.enc_request + '\r\n'
        elif self.enc_status != None:
            enc_header = 'res-hdr=0'
            enc_body = 'res-body='
            enc_req_stat = self.enc_status + '\r\n'

        if not has_body:
            enc_body = 'null-body='

        if not self.icap_headers.has_key('ISTag'):
            self.set_icap_header('ISTag', ''.join(map(
                lambda x: random.choice('ABCDIFGHIJabcdefghij1234567890'),
                xrange(32)
            )))

        if not self.icap_headers.has_key('Date'):
            self.set_icap_header('Date', self.date_time_string())

        if not self.icap_headers.has_key('Server'):
            self.set_icap_header('Server', self.version_string())

        enc_header_str = enc_req_stat
        for k in self.enc_headers:
            for v in self.enc_headers[k]:
                enc_header_str += k + ': ' + v + '\r\n'
        if enc_header_str != '':
            enc_header_str += '\r\n'

        body_offset = len(enc_header_str)

        if enc_header:
            enc = enc_header + ', ' + enc_body + str(body_offset)
            self.set_icap_header('Encapsulated', enc)

        icap_header_str = ''
        for k in self.icap_headers:
            for v in self.icap_headers[k]:
                icap_header_str += k + ': ' + v + '\r\n'
                if k.lower() == 'connection' and v.lower() == 'close':
                    self.close_connection = True
                if k.lower() == 'connection' and v.lower() == 'keep-alive':
                    self.close_connection = False

        icap_header_str += '\r\n'

        self.wfile.write(
            self.icap_response + '\r\n' +
            icap_header_str + enc_header_str
        )


    def parse_request(self):
        """Parse a request (internal).

        The request should be stored in self.raw_requestline; the results
        are in self.command, self.request_uri, self.request_version and
        self.headers.

        Return True for success, False for failure; on failure, an
        error is sent back.
        """
        self.command = None
        self.request_version = version = 'ICAP/1.0'

        # Default behavior is to leave connection open
        self.close_connection = False

        requestline = self.raw_requestline.rstrip('\r\n')
        self.requestline = requestline

        words = requestline.split()
        if len(words) != 3:
            raise ICAPError(400, "Bad request syntax (%r)" % requestline)

        command, request_uri, version = words

        if version[:5] != 'ICAP/':
            raise ICAPError(400, "Bad request protocol, only accepting ICAP")

        if command not in  ['OPTIONS', 'REQMOD', 'RESPMOD']:
            raise ICAPError(501, "command %r is not implemented" % command)

        try:
            base_version_number = version.split('/', 1)[1]
            version_number = base_version_number.split(".")
            # RFC 2145 section 3.1 says there can be only one "." and
            #   - major and minor numbers MUST be treated as
            #      separate integers;
            #   - ICAP/2.4 is a lower version than ICAP/2.13, which in
            #      turn is lower than ICAP/12.3;
            #   - Leading zeros MUST be ignored by recipients.
            if len(version_number) != 2:
                raise ValueError
            version_number = int(version_number[0]), int(version_number[1])
        except (ValueError, IndexError):
            raise ICAPError(400, "Bad request version (%r)" % version)

        if version_number != (1, 0):
            raise ICAPError(505, "Invalid ICAP Version (%s)" % base_version_number)

        self.command, self.request_uri, self.request_version = command, request_uri, version

        # Examine the headers and look for a Connection directive
        self.headers = self._read_headers()

        conntype = self.headers.get('connection', [''])[0]
        if conntype.lower() == 'close':
            self.close_connection = True

        self.encapsulated = {}
        if self.command in ['RESPMOD', 'REQMOD']:
            for enc in self.headers.get('encapsulated', [''])[0].split(','):
                # TODO: raise ICAPError if Encapsulated is malformed or empty
                k,v = enc.strip().split('=')
                self.encapsulated[k] = int(v)

        self.preview = self.headers.get('preview', [None])[0]
        self.allow = map(lambda x: x.strip(), self.headers.get('allow', [''])[0].split(','))

        if self.command == 'REQMOD':
            if self.encapsulated.has_key('req-hdr'):
                self.enc_req = self._read_request()
                self.enc_req_headers = self._read_headers()
            if self.encapsulated.has_key('req-body'):
                self.has_body = True
        elif self.command == 'RESPMOD':
            if self.encapsulated.has_key('req-hdr'):
                self.enc_req = self._read_request()
                self.enc_req_headers = self._read_headers()
            if self.encapsulated.has_key('res-hdr'):
                self.enc_res_status = self._read_status()
                self.enc_res_headers = self._read_headers()
            if self.encapsulated.has_key('res-body'):
                self.has_body = True
        # Else: OPTIONS. No encapsulation.

        # Parse service name
        # TODO: document "url routing"
        self.servicename = urlparse.urlparse(self.request_uri)[2].strip('/')

    def handle(self):
        """Handles a connection

        Since we support Connection: keep-alive, moreover this is the
        default behavior, one connection may mean multiple ICAP
        requests.
        """
        self.close_connection = False
        while not self.close_connection:
            self.handle_one_request()

    def handle_one_request(self):
        """Handle a single HTTP request.

        You normally don't need to override this method; see the class
        __doc__ string for information on how to handle specific HTTP
        commands such as GET and POST.

        """

        # Initialize handler state
        self.enc_req = None
        self.enc_req_headers = {}
        self.enc_res_status = None
        self.enc_res_headers = {}
        self.has_body = False
        self.servicename = None
        self.encapsulated = {}
        self.ieof = False
        self.eob = False
        self.methos = None
        self.preview = None
        self.allow = set()

        self.icap_headers = {}
        self.enc_headers = {}
        self.enc_status = None # Seriously, need better names
        self.enc_request = None

        self.icap_response_code = None

        try:
            self.raw_requestline = self.rfile.readline(65537)

            if not self.raw_requestline:
                self.close_connection = True
                return

            self.parse_request()

            mname = self.servicename + '_' + self.command
            if not hasattr(self, mname):
                raise ICAPError(404)

            method = getattr(self, mname)
            if not callable(method):
                raise ICAPError(404)

            method()
            self.wfile.flush()
            self.log_request(self.icap_response_code)
        except socket.timeout, e:
            self.log_error("Request timed out: %r", e)
            self.close_connection = 1
        except ICAPError, e:
            self.send_error(e.code, e.message)
        except:
            self.send_error(500)

    def send_error(self, code, message=None):
        """Send and log an error reply.

        Arguments are the error code, and a detailed message.
        The detailed message defaults to the short entry matching the
        response code.

        This sends an error response (so it must be called before any
        output has been generated), logs the error, and finally sends
        a piece of HTML explaining the error to the user.

        """

        try:
            short, long = self._responses[code]
        except KeyError:
            short, long = '???', '???'
        if message is None:
            message = short
        explain = long
        self.log_error("code %d, message %s", code, message)

        # No encapsulation
        self.enc_req = None
        self.enc_res_stats = None

        self.set_icap_response(code) # TODO: message
        self.set_icap_header('Connection', 'close') # TODO: why?
        self.send_headers()

    def send_enc_error(self, code, message=None, body='', contenttype='text/html'):
        """Send an encapsulated error reply.

        Arguments are the error code, and a detailed message.
        The detailed message defaults to the short entry matching the
        response code.

        This sends an encapsulated error response (so it must be called
        before any output has been generated), logs the error, and
        finally sends a piece of HTML explaining the error to the user.
        """

        try:
            short, long = self._responses[code]
        except KeyError:
            short, long = '???', '???'
        if message is None:
            message = short
        explain = long

        # No encapsulation
        self.enc_req = None

        self.set_icap_response(200) # TODO: message
        self.set_enc_status('HTTP/1.1 %s %s' % (str(code), message))
        self.set_enc_header('Content-Type', contenttype)
        self.set_enc_header('Content-Length', str(len(body)))
        self.send_headers(has_body=True)
        if len(body) > 0:
            self.write_chunk(body)
        self.write_chunk('')

    def log_request(self, code='-', size='-'):
        """Log an accepted request.

        This is called by send_response().
        """

        self.log_message('"%s" %s %s',
                         self.requestline, str(code), str(size))

    def log_error(self, format, *args):
        """Log an error.

        This is called when a request cannot be fulfilled.  By
        default it passes the message on to log_message().

        Arguments are the same as for log_message().

        XXX This should go to the separate error log.
        """

        self.log_message(format, *args)

    def log_message(self, format, *args):
        """Log an arbitrary message.

        This is used by all other logging functions.  Override
        it if you have specific logging wishes.

        The first argument, FORMAT, is a format string for the
        message to be logged.  If the format string contains
        any % escapes requiring parameters, they should be
        specified as subsequent arguments (it's just like
        printf!).

        The client ip address and current date/time are prefixed to every
        message.
        """

        sys.stderr.write("%s - - [%s] %s\n" %
                         (self.client_address[0],
                          self.log_date_time_string(),
                          format%args))

    def version_string(self):
        """Return the server software version string."""
        return self._server_version + ' ' + self._sys_version

    def date_time_string(self, timestamp=None):
        """Return the current date and time formatted for a message header."""
        if timestamp is None:
            timestamp = time.time()
        year, month, day, hh, mm, ss, wd, y, z = time.gmtime(timestamp)
        s = "%s, %02d %3s %4d %02d:%02d:%02d GMT" % (
                self._weekdayname[wd],
                day, self._monthname[month], year,
                hh, mm, ss)
        return s

    def log_date_time_string(self):
        """Return the current time formatted for logging."""
        now = time.time()
        year, month, day, hh, mm, ss, x, y, z = time.localtime(now)
        s = "%02d/%3s/%04d %02d:%02d:%02d" % (
                day, self._monthname[month], year, hh, mm, ss)
        return s

    def address_string(self):
        """Return the client address formatted for logging.

        This version looks up the full hostname using gethostbyaddr(),
        and tries to find a name that contains at least one dot.
        """

        host, port = self.client_address[:2]
        return socket.getfqdn(host)

    def no_adaptation_required(self):
        """Tells the client to leave the message unaltered

        If the client allows 204, or this is a preview request than
        a 204 preview response is sent. Otherwise a copy of the message
        is returned to the client.
        """
        if '204' in self.allow or self.preview != None:
            # We MUST read everything the client sent us
            if self.has_body:
                while True:
                    if self.read_chunk() == '':
                        break
            self.set_icap_response(204)
            self.send_headers()
        else:
            # We have to copy everything,
            # but it's sure there's no preview
            self.set_icap_response(200)

            self.set_enc_status(' '.join(self.enc_res_status))
            for h in self.enc_res_headers:
                for v in self.enc_res_headers[h]:
                    self.set_enc_header(h, v)

            if not self.has_body:
                self.send_headers(False)
                self.log_request(200)
                return

            self.send_headers(True)
            while True:
                chunk = self.read_chunk()
                self.write_chunk(chunk)
                if chunk == '':
                    break

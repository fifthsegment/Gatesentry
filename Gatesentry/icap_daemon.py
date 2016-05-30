#!/usr/bin/env python
import sys
import time
from daemon import Daemon
import os
import os.path
import random
import SocketServer
import zlib
import ConfigParser
from pyicap import *

config = ConfigParser.ConfigParser()
# Load filter into memory
filteredwords = []
# Load error file template into memory
error_file_loc = ""
naughtiness_score = 10

class compression:
    def __init__(self):
        self.success = False
        self.error = None

    def deflate(self, raw, compress_type='none'):
        data = None
        # print "Starting decompresser"
        print compress_type
        if (compress_type == 'deflate'):
            try:
                data = StringIO.StringIO(zlib.decompress(raw))
                self.success = True
            except zlib.error as e:
                data = raw
                self.error = e
        elif (compress_type == 'gzip'):
            try:
                # print "Trying to assign encoder"
                encoder = zlib.decompressobj(16+zlib.MAX_WBITS)
                # print "Encoder assigned"
                data = encoder.decompress(raw)
                # print "Decompress ran"
                self.success = True
            except zlib.error as e:
                data = raw
                self.error = e
                print e
        elif (compress_type == 'none'):
            self.success = True
            data = raw
        # print "Returning from compresser"
        return data
    
    def inflate(self, raw, compress_type='none'):
        data = None
        if (compress_type == 'deflate'):
            try:
                data = StringIO.StringIO(zlib.compress(raw))
                self.success = True
            except zlib.error as e:
                data = raw
                self.error = e
        elif (compress_type == 'gzip'):
            try:
                encoder = StringIO.StringIO()
                f = gzip.GzipFile(fileobj=encoder, mode='w')
                f.write(raw)
                f.close()
                data = encoder.getvalue()
                self.success = True
            except zlib.error as e:
                data = raw
                self.error = e
        elif (compress_type == 'none'):
            data = raw
            self.success = True
        return data




class ThreadingSimpleServer(SocketServer.ThreadingMixIn, ICAPServer):
    pass



class ICAPHandler(BaseICAPRequestHandler):

    def filter_OPTIONS(self):
        self.set_icap_response(200)
        self.set_icap_header('Methods', 'RESPMOD')
        self.set_icap_header('Preview', '0')
        self.send_headers(False)

    def findin(self, page):
        totalscore = 0;
        global filteredwords    
        global config    
        global naughtiness_score
        page = page.lower()
        wordsfound = ""
        for p in filteredwords:
            foundstrs = page.count(p.lower())
            totalscore = totalscore + foundstrs
            if foundstrs > 0:
                wordsfound+=p
                wordsfound+=" "
        
        print "Total score = "
        print totalscore
        print wordsfound
        sys.stderr.write( "Total score = " + str(totalscore) )
        returndata = []
        againstscore = naughtiness_score

        print "againstscore = "
        print againstscore
        if totalscore > againstscore:
            returndata.append(True)
            returndata.append(totalscore)
            returndata.append(wordsfound)
            print returndata
            return returndata;
        returndata.append(False)
        returndata.append(totalscore)
        returndata.append(wordsfound)
        print returndata
        return returndata;

    def filter_RESPMOD(self):
        # print "Incoming request"
        global error_file_loc
        self.set_icap_response(200)        
        self.set_enc_status(' '.join(self.enc_res_status))
        CHUNK_SIZE = 1
        params = {}
        content_to_analize = ['text/html', 'application/json']
        compress_type = 'none'
        if 'content-encoding' in self.enc_res_headers:
            for v in self.enc_res_headers['content-encoding']:
                content_encoding = v
            if (content_encoding in ['gzip', 'x-gzip']):
                compress_type = 'gzip'
            elif (content_encoding in ['deflate']):
                compress_type = 'deflate'
        analize = False
        if 'content-type' in self.enc_res_headers:
            for c in self.enc_res_headers['content-type']:
                i = c.replace(' ', '').split(';')
                for t in i:
                    if (t in content_to_analize):
                        analize = True
        
        if (analize == False):
            self.no_adaptation_required()
            return
        else:
            print "I need to analize this"

        print "Setting headers"
        for h in self.enc_res_headers:
            for v in self.enc_res_headers[h]:
                self.set_enc_header(h, v)
                            
                            
        if not self.has_body:
            #self.no_adaptation_required()
            print "self.has_body"
            self.send_headers(False)
            self.set_icap_response(200)
            return                              

        if self.preview:
            self.cont()

        # print "CHUNKER STARTING"
        chunks = []
        while True:
            print "READING CHUNKS"
            chunk = self.read_chunk()
            chunks.append(chunk)
            if (len(chunk) > CHUNK_SIZE):
                CHUNK_SIZE = len(chunk)
            if chunk == '':
                break

        print "Joining CHUNKS"
        data = ''.join(chunks)
        print "Data length " + str(len(data))
        if (len(data) > 0):
            if (analize):
                print "Begining analysis"
                comObj = compression()
                print "Opened up compression object"
                data_decompressed = comObj.deflate(data, compress_type)
                print "Data decompressed"
                params['data'] = data_decompressed
                changed = data_decompressed
                print "Trying to find in string"
                returneddata = self.findin(changed) 
                print returneddata
                print "Results here = "
                print returneddata[0]
                if returneddata[0] == True :
                    # print "Found changed"
                    self.error = "Incorrect String found"
                    self.set_icap_response(403)
                    ## self.send_error(403, "Incorrect string found")
                    # with open(error_file_loc, 'r') as myfile:
                        # data_file=myfile.read()
                    ## print str(returneddata[2])
                    ## data_file.replace("%E", returneddata[2])
                    # self.send_enc_error( 403, 'Prohibited', data_file.replace("%E", returneddata[2]), 'text/html')
                else:
                    self.set_icap_response(200)
                self.send_headers(True)                 
                data_compressed = data
                self.write_chunk(data)
                #chunks = [data_compressed[i:i+CHUNK_SIZE] for i in range(0, len(data_compressed), CHUNK_SIZE)]
                #for chunk in chunks:
                #   self.write_chunk(chunk)
                self.write_chunk('')
            else:
                self.set_icap_response(200)
                self.write_chunk(data)
                self.write_chunk('')
        else:
            # print "LAST ELSE"
            self.no_adaptation_required()
            # self.set_icap_response(200)
            # self.write_chunk(data)
            # self.write_chunk('')

        # self.no_adaptation_required()


class ICAPServer(object):
    def run(self):
        global filteredwords
        global error_file_loc
        print "Running ICAP Server"
        global config
        # print configfile
        filter_file_location = config.get('system','gs_filteredwords')
        error_file_loc = config.get('system', 'error_file_location')
        filteredwords = [line.strip() for line in open(filter_file_location)]

        # print filteredwords
        # self.pidfile = pidfile
        port = int(config.get('system','icap_port'))
        print "Starting Server"
        server = ThreadingSimpleServer(('', port), ICAPHandler)
        try:
            while 1:
                server.handle_request()
        except KeyboardInterrupt:
            print "Finished"

class ICAPDaemon(Daemon):
    def run(self):
       print "Daemon part"
       # Or simply merge your code with MyDaemon.
       icapserver = ICAPServer()
       print "Running code"
       icapserver.run()

if __name__ == "__main__":
    # global config
    configfile = "/etc/gatesentry/icap_server.cfg"
    if len(sys.argv) == 3:
        configfile = sys.argv[2]
    else:
        print "I didn't get the config file location as the 2nd Argument."
        print "I'm using default config file location, which is "+configfile
    if os.path.isfile(configfile) != True:
        print "Config file not found at " + configfile
        print sys.exit()
    config.read(configfile)
    print "====================="
    pid_file = config.get('system', 'pid_file')
    log_file = -1
    try:
        log_file = config.get('system', 'log_file')
    except ConfigParser.NoOptionError:
        log_file = -1
    if log_file == -1 :
        daemon = ICAPDaemon( str(pid_file) )
    else:
        print "Outputs will be logged to " + str(log_file)
        daemon = ICAPDaemon( str(pid_file) ,'/dev/null' ,log_file,log_file)
    if len(sys.argv) == 2 or len(sys.argv) == 3:
            if 'start' == sys.argv[1]:
                    config_dir = str(config.get('system', 'gs_config_dir'));
                    with open(config_dir + "/" + "config.cfg" , 'r') as f:
                        first_line = f.readline()
                    naughtiness_score = int(first_line)
                    print "New score "+ str(naughtiness_score)
                    print "Starting daemon with pid file " + str(pid_file)
                    daemon.start()
            elif 'stop' == sys.argv[1]:
                    daemon.stop()
                    if log_file != -1:
                        print "Removing log files"
                        os.remove(log_file)
            elif 'version' == sys.argv[1]:
                        print "Version = 1.0"
            elif 'restart' == sys.argv[1]:
                    daemon.restart()
            else:
                    print "Unknown command"
                    sys.exit(2)
            sys.exit(0)
    else:
            print "usage: %s start|stop|restart" % sys.argv[0]
            sys.exit(2)
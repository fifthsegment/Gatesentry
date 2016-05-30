#!/usr/bin/python -ttu
# -*- coding: utf8 -*-
'''
Credits to:
  Tamas Laszlo Fabian - pyicap Code
  Yuan-Yi Chang - gzip deflate code
  Allan GooD - providing the skeleton of this server
'''
__author__ = "Abdullah Irfan"
__copyright__ = "Copyleft/GPL v2 2014 Abdullah Irfan"

__revision__ = "$Id$"
__version__ = "1.0"

import os
import sys
import gzip
import zlib
import random
import StringIO
import SocketServer
import ConfigParser
from pyicap import *

plugin_info = {}

def termhandler(signum, frame):
    if (signum == 15):
        raise SystemExit(0)
    sys.exit(0)
    
class compression:
    def __init__(self):
        self.success = False
        self.error = None

    def deflate(self, raw, compress_type='none'):
        data = None
        if (compress_type == 'deflate'):
            try:
                data = StringIO.StringIO(zlib.decompress(raw))
                self.success = True
            except zlib.error as e:
                data = raw
                self.error = e
        elif (compress_type == 'gzip'):
            try:
                encoder = zlib.decompressobj(16+zlib.MAX_WBITS)
                data = encoder.decompress(raw)
                self.success = True
            except zlib.error as e:
                data = raw
                self.error = e
        elif (compress_type == 'none'):
            self.success = True
            data = raw
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

def import_plugins(path):
    import os
    list_of_plugins = {}
    for (dirpath, dirnames, filenames) in os.walk(path):
        for filename in filenames:
            if (filename[-3:] == '.py'):
                list_of_plugins[filename] = os.sep.join([dirpath, filename])
    for module in list_of_plugins.keys():
        try:
            execfile(list_of_plugins[module])
        except NameError as e:
            pass

def default_html():
    f = open(templates_dir + '/default.html', 'rU')
    data = ''.join(f.readlines())
    f.close
    return data
    
class ThreadingSimpleServer(SocketServer.ThreadingMixIn, ICAPServer):
    pass

	
filteredwords = []


class ICAPHandler(BaseICAPRequestHandler):
        


    def filter_OPTIONS(self):
         self.set_icap_response(200)
         self.set_icap_header('Methods', 'REQMOD')
         self.set_icap_header('Max-Connections', '10')
         self.set_icap_header('Service', 'PyICAP Server 1.0')
         self.send_headers(False)

    def filter_REQMOD(self):
        # Things that can be handled here:
        #  - Headers checks (Username / IP / User-Agent / etc)
        #  - URL filter
        #  - POST content scanner
        self.set_icap_response(200)
        
        self.set_enc_request(' '.join(self.enc_req))
        for h in self.enc_req_headers:
            for v in self.enc_req_headers[h]:
                self.set_enc_header(h, v)
	print self
        # Copy the request body (in case of a POST for example)
        if not self.has_body:
            self.send_headers(False)
            return
        self.send_headers(True)
        while True:
            chunk = self.read_chunk()
            self.write_chunk(chunk)
            if chunk == '':
                break
				
				
    def findin(self, page):
        totalscore = 0;
        global filteredwords		
        page = page.lower()
        for p in filteredwords:
            totalscore = totalscore + page.count(p.lower())
        if totalscore > 10:
			return True;
        return False;
	


    def filter_RESPMOD(self):
        self.set_icap_response(200)        
        self.set_enc_status(' '.join(self.enc_res_status))

        print self
       
        CHUNK_SIZE = 1
        # Things that can be handled here:
        #  - Headers checks (Username / IP / User-Agent / etc)
        #  - URL filter
        #  - Content scanner/filter
        
        params = {}
        content_to_analize = ['text/html', 'application/json']
        plugin_to_execute = []
        
        try:    
            params['icap_headers'] = self.headers
            params ['request_headers'] = self.enc_req_headers

            compress_type = 'none'
            if 'content-encoding' in self.enc_res_headers:
                for v in self.enc_res_headers['content-encoding']:
                    content_encoding = v
                if (content_encoding in ['gzip', 'x-gzip']):
                    compress_type = 'gzip'
                elif (content_encoding in ['deflate']):
                    compress_type = 'deflate'

            plugin_out = {}
            for plugin in plugin_info.keys():
                if (plugin_info[plugin]['req_type'].lower() == 'respmod'):
                    plugin_to_execute.append(plugin)
                    for c in plugin_info[plugin]['scan_type']:
                        content_to_analize.append(c)

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

				
        

				
            for h in self.enc_res_headers:
                for v in self.enc_res_headers[h]:
                    self.set_enc_header(h, v)
					
					
            if not self.has_body:
                #self.no_adaptation_required()
                self.send_headers(False)
                self.set_icap_response(200)
                return                              

            
            if self.preview:
                self.cont()

				
            chunks = []
            while True:
                chunk = self.read_chunk()
      		#print chunk
                chunks.append(chunk)
                if (len(chunk) > CHUNK_SIZE):
                    CHUNK_SIZE = len(chunk)
                if chunk == '':
                    break

            data = ''.join(chunks)
            if (len(data) > 0):
		self.set_icap_response(200)
		self.write_chunk(data)
		self.write_chunk('')
                if (analize):
                    comObj = compression()
                    data_decompressed = comObj.deflate(data, compress_type)
                    params['data'] = data_decompressed
                    changed = data_decompressed
                    if self.findin(changed) == True :
                        self.set_icap_response(403)
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
                self.set_icap_response(200)
                self.write_chunk(data)
                self.write_chunk('')
        except:
            print(sys.exc_info())

def drop_privileges(uid_name='nobody', gid_name='nogroup'):
    import pwd, grp
    running_uid = pwd.getpwnam(uid_name).pw_uid
    running_gid = grp.getgrnam(gid_name).gr_gid
    os.setgroups([])
    os.setgid(running_gid)
    os.setuid(running_uid)
    old_umask = os.umask(077)

def mainloop(fname):
    print "Starting main loop "
    import_plugins(modules_dir)
    server = ThreadingSimpleServer(('', icap_port), ICAPHandler)
    global filteredwords
    filteredwords = [line.strip() for line in open(fname)]
    while True:
        server.handle_request()

if __name__ == "__main__":
    configfile = sys.argv[1]
    config = ConfigParser.ConfigParser()
    config.read(configfile)
    logfile = '/tmp/icap.log'
    pidfile = '/tmp/icap.pid'
    modules_dir = './modules'
    templates_dir = './templates'
    run_as_user = 'nobody'
    run_as_group = 'nogroup'
    icap_port = 1344
    icap_port = int(config.get('system','icap_port'))
    logfile = config.get('system','log_file')
    pidfile = config.get('system','pid_file')
    modules_dir = config.get('system','modules_dir')
    templates_dir = config.get('system','templates_dir')
    run_as_user = config.get('system','run_as_user')
    run_as_group = config.get('system','run_as_group')
    filter_file_location = config.get('system','gs_filteredwords')
    
    if (os.getuid() == 0):
        drop_privileges(run_as_user,run_as_group)
    
    mainloop(filter_file_location)
    '''
    try:
        pid = os.fork()
    except OSError, e:
        raise Exception, "%s [%d]" % (e.strerror, e.errno)
    if (pid == 0):
        os.setsid()
        try:
            pid = os.fork()
        except OSError, e:
            raise Exception, "%s [%d]" % (e.strerror, e.errno)
        if (pid == 0):
            log.write('ICAP Server started and runing\n')
            f.write('%s\n' % os.getpid())
            mainloop()
        else:
            if not f.closed: f.close()
            if not log.closed: log.close()
            os._exit(0)
    else:
        if not f.closed: f.close()
        if not log.closed: log.close()
    os._exit(0)
    '''


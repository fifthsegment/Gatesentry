#!/bin/env python
# -*- coding: utf8 -*-
import sys
import os
import random
import SocketServer
import zlib
import ConfigParser
from pyicap import *


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

filteredwords = []

class ICAPHandler(BaseICAPRequestHandler):

    def filter_OPTIONS(self):
        self.set_icap_response(200)
        self.set_icap_header('Methods', 'RESPMOD')
        self.set_icap_header('Preview', '0')
        self.send_headers(False)

    def findin(self, page):
        totalscore = 0;
        global filteredwords        
        page = page.lower()
        for p in filteredwords:
            totalscore = totalscore + page.count(p.lower())
        print "Total score = "
        print totalscore
        if totalscore > 10:
            return True;
        return False;

    def filter_RESPMOD(self):
        print "Incoming request"
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
            chunks.append(chunk)
            if (len(chunk) > CHUNK_SIZE):
                CHUNK_SIZE = len(chunk)
            if chunk == '':
                break


        data = ''.join(chunks)
        if (len(data) > 0):
            if (analize):
                comObj = compression()
                # print "Opened up compression object"
                data_decompressed = comObj.deflate(data, compress_type)
                # print "Data decompressed"
                params['data'] = data_decompressed
                changed = data_decompressed
                # print "Trying to find in string"
                if self.findin(changed) == True :
                    # print "Foudnd changed"
                    self.error = "Incorrect String found"
                    self.set_icap_response(403)
                    self.send_error(403)
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

        # self.no_adaptation_required()

port = 1344


configfile = sys.argv[1]
config = ConfigParser.ConfigParser()
config.read(configfile)
filter_file_location = config.get('system','gs_filteredwords')
filteredwords = [line.strip() for line in open(filter_file_location)]
# self.pidfile = pidfile
print "Starting Server"
server = ThreadingSimpleServer(('', port), ICAPHandler)
try:
    while 1:
        server.handle_request()
except KeyboardInterrupt:
    print "Finished"

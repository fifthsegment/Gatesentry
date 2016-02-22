
@extends('layout')
@section('content')
<h1>{{$title}}</h1>
        @if(Session::has('flash_notice'))
            <div id="flash_notice">{{ Session::get('flash_notice') }}</div>
        @endif
@stop
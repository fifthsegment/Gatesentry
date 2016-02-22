
@extends('layout')
@section('content')


       
<div class="col-md-4">
</div>
<div class="col-md-4">
 <h3>GateSentry Admin Panel</h3>
{{ Form::open(array('url' => 'login')) }}

 @if(Session::has('flash_notice'))
            <div id="flash_notice">{{ Session::get('flash_notice') }}</div>
        @endif
<!-- if there are login errors, show them here -->
<p>
    {{ $errors->first('email') }}
    {{ $errors->first('password') }}
</p>
<p>
{{ Form::label('email', 'Email Address') }}
<div class="input-group input-group-lg">
  <span class="input-group-addon" id="sizing-addon1">@</span>
{{ Form::text('email', Input::old('email'), array('placeholder' => 'email@email.com', 'class' => 'form-control')) }}
</div>
    
    
</p>

<p>
{{ Form::label('password', 'Password') }}
<div class="input-group input-group-lg">
  <span class="input-group-addon" id="sizing-addon1">*</span>
{{ Form::password('password', array('placeholder' => '*******', 'class' => 'form-control')) }}
</div>



</p>

<p>{{ Form::submit('Login', array('class' => 'btn btn-default pull-right')) }}</p>
{{ Form::close() }}
@stop
</div>
<div class="col-md-4">
</div>
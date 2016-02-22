@section('content')
<h4>Update admin user</h4>
@if(Session::has('flash_notice'))
<div class="alert alert-info" role="alert">

    <div id="flash_notice">{{ Session::get('flash_notice') }}
	</div>
	
</div>
@endif
<div class="well">
{{ Form::open(array('url' => 'updatePassword')) }}
<p>IMPORTANT: Before you'll be able to change your password you must first make GateSentry's filesystem <b>writable</b>.
To do that you must SSH into GateSentry, login as root, then issue the command <b>`ipe-rw`</b>. After that return to this 
page, change your password. Once your password is changed, go back to your SSH window and
 issue the command <b>`ipe-ro`</b> to GateSentry via SSH to make the filesystem readable again. </p>
Email(Login):
<p>
<div class="input-group input-group-lg">
<input type="text" name="email" value="{{$email}}" class="form-control" >
</div>
</p>
<p>
Password:
<div class="input-group input-group-lg">
<input type="password" name="pass" value="" class="form-control" >
</div>
</div>
</p>
<p class="pull-right">{{ Form::submit('Save', array('class' => 'btn btn-primary')) }}</p>
{{ Form::close() }}
@stop
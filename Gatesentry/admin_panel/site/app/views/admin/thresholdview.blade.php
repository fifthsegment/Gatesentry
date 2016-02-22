@section('content')
<h4>Update admin user</h4>
@if(Session::has('flash_notice'))
<div class="alert alert-info" role="alert">

    <div id="flash_notice">{{ Session::get('flash_notice') }}
	</div>
	
</div>
@endif
<div class="well">
<p>The current filtering strictness is set to <b>{{$threshold}}</b><br>
<i>A lower value of strictness means more content will be blocked, while 
a higher value means lesser content will be blocked.</i>
</p>
{{ Form::open(array('url' => 'updateThreshold')) }}
Threshold:
<p>
<div class="input-group input-group-lg">
<input type="text" name="threshold" value="{{$threshold}}" class="form-control" >
</div>
</p>


<p class="pull-right">{{ Form::submit('Save', array('class' => 'btn btn-primary')) }}</p>
<br>
{{ Form::close() }}
@stop

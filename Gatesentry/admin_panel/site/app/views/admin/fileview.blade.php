@section('content')
<h4>Edit Filters > {{$headingname}}</h4>
<p>{{$description}}</p>
@if(Session::has('flash_notice'))
<div class="alert alert-info" role="alert">

    <div id="flash_notice">{{ Session::get('flash_notice') }}
	<br>
	<b>IMPORTANT</b>: To allow your settings to take affect you must reload GateSentry. <a style="color:white; " href="/application?action=reload">Click here to do that now (Might take a while).</a>
	</div>
	
</div>
@endif
<div class="well">
{{ Form::open(array('url' => 'saveChanges')) }}
<input type="hidden" name="type" value="filter" >
<input type="hidden" name="name" value="{{$name}}">
<textarea name="file_contents" style="border:1px solid #999999;
    width:100%;
    margin:5px 0;
    padding:3px;
	height: 100%;
	">
{{$file_contents}}
</textarea>

<p class="pull-right">{{ Form::submit('Save', array('class' => 'btn btn-primary')) }}</p>
{{ Form::close() }}
<br>

</div>
@stop
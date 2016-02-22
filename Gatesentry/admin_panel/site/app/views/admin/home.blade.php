@section('content')
<h4>Welcome!</h4>
<hr>
<br>
<br>
<center><h5>GateSentry is <b>{{$gatesentrystatus}}</b></h5></center>
@if (isset($error))

<center><div class="alert alert-danger" role="alert">
<b>Error</b> : {{$error}} . <a href="/application?action=restart">Click here to Restart</a>
</div>
</center>
@endif

<center><br><a class="btn btn-primary" href="/getCertificates">Click here to download GateSentry's certificate</a>
<br><br>
GateSentry's proxy port : <b>8080</b>
</center>
@stop
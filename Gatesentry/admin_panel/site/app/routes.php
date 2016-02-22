<?php

/*
|--------------------------------------------------------------------------
| Application Routes
|--------------------------------------------------------------------------
|
| Here is where you can register all of the routes for an application.
| It's a breeze. Simply tell Laravel the URIs it should respond to
| and give it the Closure to execute when that URI is requested.
|
*/

Route::get('/', function()
{
	return View::make('hello');
});
/*
Route::get('users', function()
{
        return View::make('users');
});
*/

//Route::get('login', array('uses' => 'HomeController@showLogin'));

// route to process the form
Route::post('login', array('uses' => 'HomeController@doLogin'));

Route::get('login', array('as' => 'login', 'uses' => 'HomeController@showLogin'))->before('guest');

Route::get('logout', array('as' => 'logout', function () { }))->before('auth');

Route::get('profile', array('as' => 'profile', 'uses' => 'HomeController@showWelcome'))->before('auth');

Route::get('/', array('as' => 'home', 
					  'uses' => 'AdminController@showHome'

))->before('auth');

Route::get('logout', array('as' => 'logout', function () {
			try {
				Auth::Logout();
			} catch (Exception $e) {
			
			}			
			
    return Redirect::route('home')
        ->with('flash_notice', 'You are successfully logged out.');
}))->before('auth');

Route::get('/editFilters', array('as' => 'editFilters', 'uses' => 'AdminController@editFilters'))->before('auth');

Route::get('/application', array('as' => 'application', 'uses' => 'AdminController@applicationActions'))->before('auth');

Route::get('/updatePassword', array('as' => 'updatePasswordPage', 'uses' => 'AdminController@updatePasswordPage'))->before('auth');

Route::post('/updatePassword', array('as' => 'updatePassword', 'uses' => 'AdminController@updatePassword'))->before('auth');

Route::get('/updateThreshold', array('as' => 'updateThresholdPage', 'uses' => 'AdminController@updateThresholdPage'))->before('auth');

Route::post('/updateThreshold', array('as' => 'updateThreshold', 'uses' => 'AdminController@updateThreshold'))->before('auth');

Route::get('/getCertificates', array('as' => 'getCertificates','uses' => 'AdminController@getCertificate'));

Route::post('saveChanges', array('uses' => 'AdminController@saveChanges'));



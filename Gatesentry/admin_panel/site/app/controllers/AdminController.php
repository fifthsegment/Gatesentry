<?php

class AdminController extends BaseController {
	protected $layout = 'layouts.adminmaster';
	private $data = array();
	private $basepath = array();
	private $files_list = array();
	
	public function __construct()
    {
		global $files_list ;
		global $basepath;
		
		
		//Creating Files List
		$files_list['blockedSites']='bannedsitelist';
		$files_list['allowedSites']='exceptionsitelist';
		$files_list['blockedExtensions']='bannedextensionlist';
		$files_list['blockedPhrases']='bannedphraselist';
		$files_list['sslSites']='blocked';
		$files_list['sslKeywords']='blockedregex';
		$files_list['sslContentFilter']='blockedwords.txt';
		$files_list['updateThreshold']='config.cfg';
		
		$basepath['bannedsitelist'] = "/etc/dansguardian/lists/";
		$basepath['exceptionsitelist'] = "/etc/dansguardian/lists/";
		$basepath['bannedextensionlist'] = "/etc/dansguardian/lists/";
		$basepath['bannedphraselist'] = "/etc/dansguardian/lists/";
		$basepath['blockedregex'] = "/etc/squid3/squidlist/";
		$basepath['blocked'] = "/etc/squid3/squidlist/";
		$basepath['blockedwords.txt'] = "/etc/gatesentry/lists/";
		$basepath['config.cfg'] = "/etc/gatesentry/lists/config/";
		
		
		$data["description"] = "";
		$data["public_path"] = public_path();
		$year = date("Y");
		View::share('year', $year);
	}
	
	public function startDans(){
		$dansguardian = exec('sudo -u root /etc/init.d/dansguardian start');
	}
	
	public function startSquid(){
		$dansguardian = exec('sudo -u root /etc/init.d/squid3 start');
	}
	
	public function reloadServers(){
		$icapkill = exec('sudo -u root /etc/init.d/gatesentry stop');
		$icapkill = exec('sudo -u root /etc/init.d/gatesentry start');
		$squid = exec('sudo -u root /etc/init.d/squid3 reload');
		$dansg = exec('sudo -u root /etc/init.d/dansguardian reload');
	}
	
	public function get_description($str){
		$description = array();
		$description["blockedSites"] = "TIP: Add a # infront of each line to disable the entry";
		$description["allowedSites"] = "TIP: Add a # infront of each line to disable the entry";
		if (array_key_exists ($str, $description)){
			return $description[$str];
		}
		return "";
	}
	
	public function logout(){
		$files = glob('/etc/gatesentry/admin_panel/site/app/storage/sessions/*'); // get all file names
		foreach($files as $file){ // iterate files
		  if(is_file($file))
			unlink($file); // delete file
		}
	}
	
    /**
     * Make the Homepage
     */
	
	public function readFile($filename){
		global $basepath;
		$filename = $basepath[$filename].$filename;
		$myfile = file_get_contents($filename) or $myfile="";
		return $myfile;

	}
	
	public function saveFile($filename, $contents){
		global $basepath;
		exec('sudo -u root /etc/init.d/write.sh');
		$filename = $basepath[$filename].$filename;
		file_put_contents($filename,$contents);
		exec('sudo -u root /etc/init.d/read.sh');
	}
	
	
    public function showHome()
    {
	    global $data;
		global $basepath;
		
		
	    $data["username"] = Auth::user()->username;
		$squid3 =  shell_exec('/etc/init.d/squid3 status');
		$dansguardian = shell_exec('/etc/init.d/dansguardian status');
		$running = 0;
		$error = "";
		$squid3 = strpos($squid3, "squid3 is running.");
		$data["squid"]="notrunning";
		if ($squid3==0 && strlen($squid3) == 1){
			$running = $running + 1;
			$data["squid"] = "running";
		}else{
			$error = "Squid3 not working<br>";
		}
		$dansguardian = strpos($dansguardian, "dansguardian is running.");
		if ($dansguardian==0 && strlen($dansguardian) == 1){
			$running = $running + 1;
		}else{
			$error .= "Dansguardian not working";
		}
		
		if ($running == 2){
			
			$data["gatesentrystatus"] = "running";
			
		}else{
			$data["gatesentrystatus"] = "not running";
		}
		if (strcmp($error,"")){
			$data["error"] = $error;
		}
		
		
		
        $this->layout->content = View::make('admin.home',  $data);
    }
	
	public function applicationActions(){
		global $data;
		global $basepath;
		$action = Input::get('action', 'default');
		if ($action == "restart"){
			$this->startDans();
			$this->startSquid();

			return Redirect::route('application')
			->with('flash_notice', 'SUCCESS: GateSentry restarted successfully.');
		}
		
		if ($action == "reload"){
			$this->reloadServers();
			return Redirect::route('application')
			->with('flash_notice', 'SUCCESS: GateSentry reloaded successfully. Please wait for at least 60 seconds for settings to take effect.');
		}
		$data["title"] = "GateSentry restarting...";
		$this->layout->content = View::make('home',  $data);
	}
	
	public function editFilters(){
		global $basepath;
		global $files_list;
		
		$name = Input::get('name', 'blockedSites');
		$data["name"] = $name;
		$data["description"] = $this->get_description($name);
		$data["headingname"] = $name;
		if (array_key_exists ($name, $files_list)){
			$filename = $files_list[$name];
			
			$data["file_contents"] = $this->readFile($filename);
			$this->layout->content = View::make('admin.fileview',  $data);
		}else{
			return "Error: File not found";
		}
	}
	
	public function getCertificate(){
		
		$pathToFile = "/etc/squid3/certs/myCA.der";
	    try{
			return Response::download($pathToFile);
		}
		catch(Exception $e){
			print_r($e);
		}
		//return 'ab';
	}
	
	public function makeWritable(){
	 return exec('sudo -u root /etc/init.d/write.sh');
	}
	
	public function makeReadable(){
	 return exec('sudo -u root /etc/init.d/read.sh');
	}

	public function saveChanges(){
		global $basepath;
		global $files_list;
		
		$type = Input::get('type', 'undefined');
		if ($type=="filter"){
			if (array_key_exists (Input::get('name'), $files_list)){
				$filename = $files_list[Input::get('name')];
				$x = Input::get('file_contents');
				$str = str_replace(PHP_EOL, '\n', $x);
					
				$this->saveFile($filename, Input::get('file_contents'));
			}
			$queryToAdd = array('name' => Input::get('name'));
			$currentQuery = Input::query();
			// Merge our new query parameters into the current query string
			$query = array_merge($queryToAdd, $currentQuery);
			

			//return $str;
			return Redirect::route('editFilters', $query)
			->with('flash_notice', 'File saved succesfully.');
		}
		return "Error : Type undefined";
	}
	
	public function updatePassword(){
		$password = Input::get('pass');
		$email = Input::get('email');
		$user = User::find(1);
		$validator = Validator::make(
			array(
				
				'password' => $password,
				'email' => $email
			),
			array(
				'password' => 'required|min:6',
				'email' => 'required|email|'
			)
		);
		
		if ($validator->passes())
		{

			$user->password = Hash::make($password);
			$user->email = $email;
			try {
				$user->save();
			} catch (Exception $e) {
				return Redirect::route('updatePassword')
				->with('flash_notice', '<b>Error</b>: Update failed due to a locked filesystem. To update your password you must first unlock the filesystem by issuing the `ipe-rw` command via SSH, then come back to this page and change your password.');
			}


			return Redirect::route('updatePassword')
			->with('flash_notice', 'User updated successfully.');
		}else {
			$messages =  $validator->failed();
			return Redirect::route('updatePassword')
			->with('flash_notice', '<b>Error</b>: Update failed');
		}
			
		
		

		
		
	}
	
	public function updatePasswordPage(){
		
		$user = User::find(1);
		//$this->makeWritable();
		$data["email"] = $user->email;
		$this->layout->content = View::make('admin.userview',  $data);
		//$this->makeReadable();
		//return "Done";
	}
	
	public function updateThreshold(){
		$threshold = Input::get('threshold');
		$this->saveFile('config.cfg', $threshold);
		return Redirect::route('updateThreshold')
			->with('flash_notice', '<b>Saved!</b> Value Updated successfully. However for updated settings to take effect you must reload/restart GateSentry.');
	}
	
	public function updateThresholdPage(){
		
		$user = User::find(1);

		$data["threshold"] = $this->readFile('config.cfg');
		$this->layout->content = View::make('admin.thresholdview',  $data);
		//return "Done";
	}
}
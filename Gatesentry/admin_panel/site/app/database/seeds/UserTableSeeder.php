<?php

class UserTableSeeder extends Seeder
{

public function run()
{
    DB::table('users')->delete();
    User::create(array(
        'name'     => 'Abdullah Irfan',
        'username' => 'abdullah',
        'email'    => 'abdullahirfanwork@gmail.com',
        'password' => Hash::make('letmein'),
    ));
}

}

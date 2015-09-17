#!/usr/bin/env node

var program = require('commander');

var superagent = require('superagent');
var randomstring = require('randomstring');
var async = require('async');

var tidepoolPlatform = require('tidepool-platform-client');
var storage = require('./node_modules/tidepool-platform-client/lib/inMemoryStorage');

var CARELINK_CLI_PATH = './node_modules/tidepool-uploader/lib/carelink/cli/csv_loader.js'
var INSULET_CLI_PATH = './node_modules/tidepool-uploader/lib/insulet/cli/ibf_loader.js';
var UPLOAD_CONFIG_PATH = './node_modules/tidepool-uploader/config/local.sh'

var pjson = require('./package.json');

var intro = 'Load Test CLI:';

function initPlatform(config, cb) {
  var consoleLog = { info: console.log, warn: console.log };

  var client = tidepoolPlatform(
    {
      host: config.host,
      metricsSource : pjson.name,
      metricsVersion : pjson.version
    },
    {
      superagent : superagent,
      log : consoleLog,
      localStore: storage()
    }
  );
  client.initialize(function(err){
    return cb(err, client);
  });
}

function createAccount(client, cb){

  var newPassword = function(){
    return randomstring.generate({
      length: 8
    });
  };
  var newUsername = function(){
    var un = randomstring.generate({
      length: 6,
      readable: true,
      charset: 'alphabetic'
    });
    return un+"+skipit@tidepool.ninja";
  };

  var un = newUsername();

  var user = {
    id: null,
    username: un ,
    emails: [un],
    password: newPassword(),
    profile: {fullName:un,patient:{birthday:'1900-01-01',diagnosisDate:'1900-01-01'}}
  };

  async.waterfall([
    function(callback) {
      //Create Account
      console.log('adding ',user);
      client.signup(user, {}, function(err, details){
        user.id = details.userid;
        callback(err, details.userid);
      });
    },
    function(userid, callback) {
      client.addOrUpdateProfile(userid, user.profile, function(err, details){
        callback(err);
      });
    }
  ], function (err, result) {
    //Done and dusted with no errors we hope
    return cb(err, user);
  });
}

function uploadData(details, cb){

  var exec = require('child_process').exec, child;
  var start = new Date();

  child = exec('node '+INSULET_CLI_PATH+' -f '+details.file+' -u '+details.username+' -p '+details.password,
    function (error, stdout, stderr) {
      if (error !== null) {
        console.log('error uploading data: ' + error);
        return cb(error);
      }
      console.log('upload took',  new Date() - start, 'millis');
      return cb();
  });
}

function readData(client, userid, cb){
  var start = new Date();
  client.getDeviceDataForUser(userid, function(err, resp){
    if (err) {
      console.log('error reading data: ', err);
    }
    console.log('download took', new Date() - start ,'millis');
    return cb(err);
  });
}

function runForOne(client, cb){
  async.waterfall([
      function(callback) {
        console.log('creating account ...');
        createAccount(client, function(err, account){
          //console.log('account', account);
          callback(err, account);
        });
      },
      function(account, callback) {
        console.log('uploading data ...');
        var details = {
          username: account.username,
          password: account.password,
          file: '/Users/jhbate/Documents/Tidepool/src/data/2015.03.18_MM.ibf'
        };

        uploadData(details, function(err){
          callback(err, account);
        });
      },
      function(account, callback) {
        console.log('reading data ...');
        readData(client, account.id, function(err){
          callback(err, account);
        });
      },
      function(account, callback) {
        console.log('uploading data AGAIN ...');
        var details = {
          username:account.username,
          password:account.password,
          file: '/Users/jhbate/Documents/Tidepool/src/data/2015.03.18_MM.ibf'
        };

        uploadData(details, function(err){
          callback(err, account);
        });
      },
      function(account, callback) {
        console.log('reading data AGAIN ...');
        readData(client, account.id, function(err){
          callback(err);
        });
      }
    ], function (err, result) {
      //Done and dusted with no errors we hope
      return cb(err);
    });
}

/**
 * Our CLI that does the work to load the specified raw csv data
 *
 **/
program
  .version('0.0.1')
  .option('-u, --username [user]', 'username')
  .option('-p, --password [pw]', 'password')
  .option('-n, --number [num]', 'simultaneous users to simulate load for', 5)
  .parse(process.argv);

console.log(intro, 'Starting load test ...');

if(program.username && program.password) {

  initPlatform({host: process.env.API_URL },function(err, client){
    if (err){
      console.log('error init platform: ',err);
      return
    }

    var users = [];

    for (var i = program.number - 1; i >= 0; i--) {
      users.push(i);
    };

    async.each(users, function(user, callback) {
      runForOne(client, callback);
    }, function(err){
      if( err ) {
        console.log('A test run failed');
      } else {
        console.log('All tests run');
      }
      process.exit();
    });
  });

}else{
  program.help();
}
#!/usr/bin/env node

var program = require('commander');

var superagent = require('superagent');
var randomstring = require('randomstring');
var async = require('async');
var fs = require('fs');

var tidepoolPlatform = require('tidepool-platform-client');
var storage = require('./node_modules/tidepool-platform-client/lib/inMemoryStorage');

var CARELINK_CLI_PATH = './node_modules/tidepool-uploader/lib/carelink/cli/csv_loader.js';
var INSULET_CLI_PATH = './node_modules/tidepool-uploader/lib/insulet/cli/ibf_loader.js';

var ACCOUNTS_FILE = 'load_test_accounts_';

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
    return un+process.env.AUTH_EMAIL;
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

    var basics = {
      id: user.id,
      username: user.username,
      password: user.password,
      uploads: [],
      downloads: []
    };

    return cb(err, basics);
  });
}

function uploadInsuletData(details, cb){

  var exec = require('child_process').exec, child;
  var start = new Date();

  child = exec('node '+INSULET_CLI_PATH+' -f '+process.env.UPLOAD_IBF_FILE+' -u '+details.username+' -p '+details.password,
    function (error, stdout, stderr) {
      var finish = new Date();
      if (error !== null) {
        console.log('error uploading data: ', error);
      }
      return cb(error, {started: start , finished: finish, elapsedMs: finish-start });
  });
}

function uploadCarelinkData(details, cb){

  var exec = require('child_process').exec, child;
  var start = new Date();

  child = exec('node '+CARELINK_CLI_PATH+' -f '+process.env.UPLOAD_CL_FILE+' -u '+details.username+' -p '+details.password,
    function (error, stdout, stderr) {
      var finish = new Date();
      if (error !== null) {
        console.log('error uploading data: ', error);
      }
      return cb(error, {started: start , finished: finish, elapsedMs: finish-start });
  });
}

function readData(client, details, cb){
  var start = new Date();
  client.getDeviceDataForUser(details.userid, function(err, resp){
    var finish = new Date();
    if (err !== null) {
      console.log('error reading data: ', err);
    }
    return cb(err, {started: start , finished: finish, elapsedMs: finish-start });
  });
}

function runForOne(client, cb){

  console.log('starting user test run ...');

  async.waterfall([
      function(callback) {
        createAccount(client, function(err, account){
          callback(err, account);
        });
      },
      function(account, callback) {
        var userDetails = {
          userid: account.id,
          username: account.username,
          password: account.password
        };

        uploadInsuletData(userDetails, function(err, upload){
          account.uploads.push(upload);
          callback(err, account);
        });
      },
      function(account, callback) {

        var userDetails = {
          userid:account.id
        };

        readData(client, userDetails, function(err, download){
          account.downloads.push(download);
          callback(err, account);
        });
      },
      function(account, callback) {
        var userDetails = {
          userid: account.id,
          username: account.username,
          password: account.password
        };

        uploadInsuletData(userDetails, function(err, upload){
          account.uploads.push(upload);
          callback(err, account);
        });
      },
      function(account, callback) {
        var userDetails = {
          userid:account.id
        };
        readData(client, userDetails, function(err, download){
          account.downloads.push(download);
          callback(err, account);
        });
      }
    ], function (err, result) {
      //Done and dusted with no errors we hope
      console.log('finished user test run');
      return cb(err, result);
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
  .option('-s, --simultaneous <n>', 'number of simultaneous users to simulate load for', parseInt)
  .option('-c, --cycles <n>', 'number of cyles to run the test for', parseInt)
  .parse(process.argv);

console.log(intro, 'Starting load test ...');

if(program.username && program.password) {

  initPlatform({host: process.env.API_URL },function(err, client){
    if (err){
      console.log('error init platform: ',err);
      return
    }

    //Hack but good enough for now
    var users = [];
    for (var i = program.simultaneous - 1; i >= 0; i--) {
      users.push(i);
    };

    var count = 0;
    var report = {
      runs:[]
    };

    async.whilst(
        function () { return count < program.cycles; }, //how many cycles of the test are we running
        function (cb) {
          count++;
          console.log('starting testing cycle ', count ,'out of',program.cycles);
          var run = { runNumber: count, started: new Date(), finished: '', users:[] };

          async.each(users, function(user, cb) {  //start test run for all simultaneous users
            runForOne(client, function(err, runDetails){
              run.users.push(runDetails);
              cb(err);
            });
          }, function(err){
            if( err ) {
              console.log('A test run failed');
              cb(err);
            } else {
              console.log('finished testing cycle',count, 'out of',program.cycles);
            }
            run.finished = new Date();
            report.runs.push(run);
            cb();
          });
        },
        function (err) {
          var when = new Date().toUTCString();
          fs.appendFileSync(ACCOUNTS_FILE+when+'.json', JSON.stringify(report));
          process.exit();
        }
    );
  });

}else{
  program.help();
}
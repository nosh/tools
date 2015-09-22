# tidepool-loadtest

Cmd-line testing tool that simulates x number of users simultaneously using the platform for x number of cycles through the process

## Getting Started
Install the module with: `npm install`

## Documentation

```
> source config.sh
```

```
Usage: load_test [options]

  Options:

    -h, --help              output usage information
    -V, --version           output the version number
    -u, --username [user]   username
    -p, --password [pw]     password
    -s, --simultaneous <n>  number of simultaneous users to simulate load for
    -c, --cycles <n>        number of cyles to run the test for
```

```
> node load_test.js -u myaccount@testing.org -p mypassword -s 10 -c 3
```

## Release History

* 0.1.0 -- 2015-09-20 -- initial creation, by Jamie Bate

## License
 == BSD2 LICENSE ==
 Copyright (c) 2015, Tidepool Project

 This program is free software; you can redistribute it and/or modify it under
 the terms of the associated License, which is identical to the BSD 2-Clause
 License as published by the Open Source Initiative at opensource.org.

 This program is distributed in the hope that it will be useful, but WITHOUT
 ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS
 FOR A PARTICULAR PURPOSE. See the License for more details.

 You should have received a copy of the License along with this program; if
 not, you can obtain one from Tidepool Project at tidepool.org.
 == BSD2 LICENSE ==



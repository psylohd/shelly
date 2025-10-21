# Shelly
v 0.0.1

A wrapper for socat and nc with reverse shell generation, a http server for payload uploads.

Features

*   Basic shell mode using nc with commands to automate shell upgrades 
    *   :upgrade - python3 psuedoshell  !WIP (linux only right now)
    *   :socat - upgrade to a socat reverse shell
    *   :quit
*   Configurable revserse shell templates
*   Downloads common tools (eg: linpeas.sh) and puts into the “toolbox” which can be easily used in basic nc mode !WIP

TODO:

*   complete toolbox to automate tool upload
*   generate example config on first run
*   prompt user for architecture (unless we can infer it from initial reverse shell download)
*   fetch latest versions of tools intead of a fixed version
*   revshell Encoding
*   fully interactive wrapper with commands ??? See termux for example ???
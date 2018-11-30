# sharebase
API and command line tools to interact with Hyland's ShareBase platform

# Sample output

Sample output from `sb -h` command:

```
Usage of /home/sean/gopath/bin/sb:
  -Config string
        Combined configuration file (default "@/home/sean/sharebase.conf")
  -DataCenter string
        The ShareBase data center to connect to.
         (default "https://app.sharebase.com/sharebaseapi")
  -LogLevel string
        Logging level (default "Warn")
  -Password string
        The password used to connect to ShareBase.
  -Token string
        The ShareBase authentication token included in all requests.  Using a token
        precludes the use of a username and password and vice-versa.
  -Username string
        The username used to establish a connection to the data center.

Positional parameters:
  [Source] string
        The source file to read from.  Can be either a ShareBase URL or a local
        path.
  [Target] string
        The target file to write to.  Can be either a ShareBase URL or a local
        path.

All of the variables can reference a configuration file instead of an actual
value by prefixing the parameter value with an @.  For example, if the password
is in a text file, the password can be specified as:

    -Password @password_file.txt

To load the password from a file called "password_file.txt" instead of putting
the actual value on the command line.

If a single configuration file is used with the Config parameter, the parameters
within the file must each be on their own line and prefixed with the command
line argument (e.g. "DataCenter", etc.), then an equals sign ("=") and then the
parameter value, for example:

    DataCenter = https://app.sharebase.com/sharebaseapi
    Username   = MyUsername@email.com
    Password   = My Sup3r S#cret P@ssw0rd


```

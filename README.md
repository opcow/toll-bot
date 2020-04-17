# A Discord sars-cov-2 stat tracker.



| Command  | Description  | Req. Op  |
|---|---|---|
| !cov [country \| all]  | report the latest stats, defaults to 'usa'  | no  |
| !reaper [channel id \| off] | periodically report the death toll to the channel given or currren channel  | yes  |
| !op \<user id\> | add a user to the operators  | yes  |
| !deop [user id] | remove a user/self from the operators  | yes  |
| !delmsg \<server id\> \<message id\> | delete a message  | no  |
| !config | print the current config via direct message | yes  |
| !quit  | kill the bot  | yes  |

    -t [discord autentication token]
    -r [rapidapi autentication token]
    -c [cron spec] (e.g. "1 */2 * * *" will post reports 1 minute after even hours)
    -i [comma separated string of initial channels to report to]
	-o [comma separated string of operators for the bot]

The following environment variables can be used instead of the above command line options. Any option given on the command line will override the corresponding environment variable. 

    DISCORDTOKEN
    RAPIDAPITOKEN
    TBCHANS
    TBCRONSPEC
    TBOPS

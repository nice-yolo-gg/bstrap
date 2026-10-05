If you ever find yourself on a server and that server is specifically Ubuntu 24 LTS and you learn that apparently `apt-get install sudo` is a reasonable full command
then perhaps you want to go ahead and automate part of the process.

This script aims to do that for me. Perhaps someone that isn't me will learn. Hopefully it is not applied outright to non-suitable situations.

### What is does:

1. Installs stuff in reasonable order (for a server with 8GB ram and 4vCPU. Possible with less it will choke a bit i'd personally split it up if so.

2. speedily makes sure you wont have to keep logging in as root, the use of the install command reliably achieves correct settings for a non root account for you. Two, even.

3. Gets a hold of eza and gh cli even though its not as readily available as the rest of the packages. Because I like those.

4. Generates four user groups. That's something I happened to need.

5. Packs something up, I forgot what, in a 7z archive with a decent password. Obviously no point if is also displayed on line. Will delete the whole concept

### Incoming additional improvements

1. Iris is incoming. Iris is an NPC who provisions new users with linger and dbus and pam and the whole package you see.  This leads to an ability to call coding bots on the phone and other things not being broke Irish developed quickly to do some minor additional things Not things that was particularly needed but it's handy and if expanded greatly she'll be one with the script.

### Planned Additions:

1. If you try to run with a distro that doesn't have  apt, it should tell you its a waste of time

2. Configure your user accounts, apply any number under something reasonable, and then name them all by hand.

3. 

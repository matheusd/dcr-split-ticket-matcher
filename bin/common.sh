#!/bin/bash

RELEASE=0
CURRENT_COMMIT=`git log --pretty=format:'%h' -n 1`
if [[ $1 == "release" ]] ; then
    VERSION=`go run ./pkg/version/cmd release`
    RELEASE=1
else
    VERSION=`go run ./pkg/version/cmd nometa`
fi

DIRTY=""
if [[ -n $(git status -s) ]] ; then
    DIRTY=".dirty"
fi


# $1 = path to output (-o $1)
# $2 = command to build
go_build() {
    cmd="go build -v"
    if [[ $RELEASE == 1 ]] ; then
        cmd="$cmd -ldflags '-X github.com/matheusd/dcr-split-ticket-matcher/pkg/version.BuildMetadata=release.$CURRENT_COMMIT$DIRTY \
            -X github.com/matheusd/dcr-split-ticket-matcher/pkg/version.PreRelease='"
    else
        cmd="$cmd -ldflags '-X github.com/matheusd/dcr-split-ticket-matcher/pkg/version.BuildMetadata=$CURRENT_COMMIT$DIRTY'"
    fi
    cmd="$cmd -o $1 $2"

    # echo ""
    # echo $cmd
    # echo ""

    eval $cmd
    if [[ $? != 0 ]] ; then exit 1 ; fi
}

#!/usr/bin/env python3

from github3 import login, GitHub
import os
import sys
import time
import json
import re

import json
import mimetypes
import os
import requests
import glob
from git import Repo

class UploadGH(object):
    def __init__(self, repo, token=None):
        self.token = os.getenv("GITHUB_TOKEN", token)
        self.endpoint = "https://api.github.com/repos/%s" % repo

    def headers(self, extra={}):
        headers = {'Authorization': "token %s" % self.token}
        headers.update(extra)
        return headers

    def create_release(self, tag):
        path = self.endpoint + "/releases"
        params = {"tag_name": tag,
                  "name": tag,
                  "draft": False,
                  "prerelease": True,
                  "body": "Release %s" % tag}
        resp = requests.post(path, data=json.dumps(params), headers=self.headers())
        resp.raise_for_status()
        return resp

    def get_release(self, tag):
        path = self.endpoint + "/releases/tags/%s" % tag
        resp = requests.get(path)
        if resp.status_code == 404:
            resp = self.create_release(tag)
        resp.raise_for_status()
        return resp.json()

    def get_existing_asset(self, name, release_id):
        path = self.endpoint + "/releases/%s/assets" % release_id
        resp = requests.get(path)
        assets = resp.json()

        if resp.status_code < 299:
            for asset in assets:
                if asset['name'] == name:
                    return asset

        return None

    def delete_asset(self, asset):
        resp = requests.delete(asset['url'], headers=self.headers())
        resp.raise_for_status()

    def upload(self, filepath, tag, name=None,
               overwrite=True, contenttype="application/octet-stream"):
        release = self.get_release(tag)
        upload_url = release['upload_url'].split("{")[0]
        name = name or os.path.basename(filepath)
        data = open(filepath, "rb").read()

        if overwrite:
            previous_asset = self.get_existing_asset(name, release['id'])
            if previous_asset:
                self.delete_asset(previous_asset)

        if contenttype is None:
            mimetypes.init()
            contenttype = mimetypes.guess_type(filepath)[0]

        resp = requests.post(upload_url, data=data,
                             headers=self.headers({'Content-Type': contenttype}),
                             params={"name": name})

        return resp

# fixme: to be changed to a decred repo
BUILD_REPO_OWNER = "matheusd"
BUILD_REPO_REPO = "dcr-split-ticket-matcher"

RELEASE_INFO = """# Split Ticket Service & Client

This is a **beta** release of the split ticket buyer service and client. Please
read the [instructions for joining the beta](/docs/beta.md).
"""


def getVersion():
    with open("pkg/version.go") as f:
        for l in f.readlines():
            m = re.match(r'^\s+Version\s+= "(.+)"$', l)
            if m:
                return m.group(1)
    return ""


def main():
    if (not ("GITHUB_OATH_TOKEN" in os.environ)):
        print("Please define the env variable GITHUB_OATH_TOKEN with the github token")
        sys.exit(1)

    local = Repo(".")
    if local.is_dirty():
        print("Local repo is dirty. Please commit.")
        sys.exit(1)

    if local.active_branch.name != "master":
        print("Trying to deploy when not in master (%s)." % local.active_branch.name)
        sys.exit(1)

    local.remotes.origin.push()

    g = login(token=os.environ["GITHUB_OATH_TOKEN"])

    destRepo = g.repository(BUILD_REPO_OWNER, BUILD_REPO_REPO)

    version = getVersion()
    tagName = "v" + version

    hasRelease = False
    release = None
    for existRelease in destRepo.iter_releases(1):
        hasRelease = existRelease.tag_name == tagName
        release = existRelease

    if not hasRelease:
        release = destRepo.create_release(tagName, prerelease=True, body=RELEASE_INFO)
        print("Created Release %s" % tagName)
    else:
        print("Release %s already exists" % tagName)

    upload = UploadGH(BUILD_REPO_OWNER + "/" + BUILD_REPO_REPO, os.environ["GITHUB_OATH_TOKEN"])
    files = glob.glob("dist/archives/*-%s*" % version)
    for file in files:
        print("Uploading %s" % file)
        upload.upload(file, tagName)


if __name__ == "__main__":
    main()

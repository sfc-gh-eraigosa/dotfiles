#!/usr/bin/env ruby

require 'uri'
require 'json'
require 'octokit'
require 'byebug'

# GitHubIssues work with octokit to setup and list our issues
class GitHubIssues
  def client
    client = Octokit::Client.new(:access_token => ENV['GITHUB_TOKEN'])
    user = client.user
    user.login
    client
  end
  def get_issues(project) # like github/engineering-effectiveness
    octokit = client
    # octokit.auto_paginate = true
    # issues = octokit.issues project
    search_term = "repo:\"#{project}\" is:issue is:open label:\"milestone: RC\""
    puts search_term
    issues = octokit.search_issues search_term
    issues_items = issues.items

    last_response = octokit.last_response
    while true do
      # ... process the last_response.data ...
      break unless last_response.rels[:next]
      nextissues = octokit.last_response.rels[:next].get.data
      issues_items = issues_items + nextissues.items
      last_response = last_response.rels[:next].get
    end
    issues_items
  end

  def gh_issues_to_csv(issues)
    puts issues.length
    issues.each do | issue |
      puts "\"#{issue.url}\",\"#{issue.title}\"" # body state url
    end
  end

  def run(project)
    # byebug
    # gh_issues_to_csv get_issues project
    gh_issues_to_csv get_issues(project)
  end
end

if $PROGRAM_NAME == __FILE__
  gh = GitHubIssues.new
  puts gh.run(ARGV[0])
end

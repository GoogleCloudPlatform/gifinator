/*
 * Copyright 2017 Google Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/**
 *  Frontend_checkJob
 *  Will poll the backend to see if the target job has been completed, and if
 *  so trigger a reload of the page (which should show the completed gif)
 *    @job_id       string    The ID of the job
 *    @on_complete  function  Callback to exectute once the job's final status
 *                            has been determined. The callback takes two
 *                            parameters:
 *                                 @status 0=failed, 1=succeeded, 2=waiting
 *                                 @err    null or the error object if status=0
 */

function Frontend_checkJob(job_id, on_complete) {
  _getRemoteJson("/check/"+job_id, function(http_status, data){
    if(http_status==200){
      if(data.status != null) {
        on_complete(data.status, null);
      }else{
        alert('Error retrieving status.');
      }
    }else{
      alert('Error retrieving status. Error code: '+http_status);
    }
  });
}

/**
 *  _getRemoteJson
 *  Hits a remote endpoint via JSON, and primes the supplied callback
 *
 *    @uri          string    The ID of the job
 *    @on_complete  function  Callback to exectute once the HTTP request
 *                            completes. The callback takes two parameters:
 *                              @http_status http status code
 *                              @data        de-serialized JSON object if status
 *                                           is 200, else null
 */

function _getRemoteJson(uri, on_complete) {
  var xmlhttp = new XMLHttpRequest();
  xmlhttp.onreadystatechange = function() {
    if (xmlhttp.readyState == XMLHttpRequest.DONE ) {
      if (xmlhttp.status == 200) {
        on_complete(xmlhttp.status,JSON.parse(xmlhttp.responseText));
      } else {
        on_complete(xmlhttp.status,null)
      }
    }
  }
  xmlhttp.open("GET", uri, true);
  xmlhttp.send();
}

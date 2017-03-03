/**
 * Javascript for the GifCreator webapp
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
  _getRemoteJson("/check/"+job_id, function(req_status, data){
    if(req_status){
      if(data.status != null) {
        on_complete(data.status, null);
      }else{
        alert('Error retrieving status.');
      }
    }else{
      alert('Error retrieving status. Error code: '+data);
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
 *                              @http_status 0=failed, 1=succeeded
 *                              @data        if status=0, HTTP error code
 *                                           if status=1, data object
 */

function _getRemoteJson(uri, on_complete) {
  var xmlhttp = new XMLHttpRequest();
  xmlhttp.onreadystatechange = function() {
    if (xmlhttp.readyState == XMLHttpRequest.DONE ) {
      if (xmlhttp.status == 200) {
        on_complete(1,JSON.parse(xmlhttp.responseText));
      } else {
        on_complete(0,xmlhttp.status)
      }
    }
  }
  xmlhttp.open("GET", uri, true);
  xmlhttp.send();
}
